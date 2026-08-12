import { createHash } from "node:crypto";
import {
	mkdir,
	open,
	readdir,
	readFile,
	stat,
	writeFile,
} from "node:fs/promises";
import { dirname, join } from "node:path";

const SEGMENT_PATTERN = /^segment-(\d{16})\.jsonl$/;

type WalBody<T> = {
	version: 1;
	sequence: number;
	kind: string;
	payload: T;
};

type WalEnvelope<T> = WalBody<T> & {
	length: number;
	sha256: string;
};

export type WalRecord<T = unknown> = Readonly<WalBody<T>>;

export type DurableWalOptions = {
	maxRecordBytes?: number;
	maxSegmentBytes?: number;
};

export class DurableWalStore {
	readonly #directory: string;
	readonly #maxRecordBytes: number;
	readonly #maxSegmentBytes: number;
	readonly #records: WalRecord[];
	#nextSequence: number;
	#segmentStart: number;
	#segmentSize: number;
	#closed = false;
	#appendQueue: Promise<void> = Promise.resolve();

	private constructor(
		directory: string,
		options: Required<DurableWalOptions>,
		records: WalRecord[],
		segmentStart: number,
		segmentSize: number,
	) {
		this.#directory = directory;
		this.#maxRecordBytes = options.maxRecordBytes;
		this.#maxSegmentBytes = options.maxSegmentBytes;
		this.#records = records;
		this.#nextSequence = (records.at(-1)?.sequence ?? 0) + 1;
		this.#segmentStart = segmentStart;
		this.#segmentSize = segmentSize;
	}

	static async open(
		directory: string,
		options: DurableWalOptions = {},
	): Promise<DurableWalStore> {
		const normalized = {
			maxRecordBytes: options.maxRecordBytes ?? 2 * 1024 * 1024,
			maxSegmentBytes: options.maxSegmentBytes ?? 32 * 1024 * 1024,
		};
		if (
			normalized.maxRecordBytes < 1024 ||
			normalized.maxSegmentBytes < normalized.maxRecordBytes
		) {
			throw new TypeError("WAL size limits are invalid");
		}

		await ensureDirectory(directory);

		const names = (await readdir(directory))
			.filter((name) => SEGMENT_PATTERN.test(name))
			.sort();
		const records: WalRecord[] = [];
		let expectedSequence = 1;
		let segmentStart = 1;
		let segmentSize = 0;

		for (let index = 0; index < names.length; index += 1) {
			const name = names[index];
			if (!name) continue;
			const path = join(directory, name);
			const loaded = await loadSegment(
				path,
				index === names.length - 1,
				normalized.maxRecordBytes,
			);
			for (const record of loaded.records) {
				if (record.sequence !== expectedSequence)
					throw new Error(
						`WAL sequence gap at ${record.sequence}; expected ${expectedSequence}`,
					);
				records.push(record);
				expectedSequence += 1;
			}
			segmentStart = Number(SEGMENT_PATTERN.exec(name)?.[1]);
			segmentSize = loaded.size;
		}

		if (names.length === 0) {
			const path = join(directory, segmentName(1));
			const handle = await open(path, "a", 0o600);
			await handle.sync();
			await handle.close();
		}

		return new DurableWalStore(
			directory,
			normalized,
			records,
			segmentStart,
			segmentSize,
		);
	}

	records<T = unknown>(kind?: string): WalRecord<T>[] {
		const records =
			kind === undefined
				? this.#records
				: this.#records.filter((record) => record.kind === kind);
		return records.map((record) => structuredClone(record)) as WalRecord<T>[];
	}

	async append<T>(kind: string, payload: T): Promise<WalRecord<T>> {
		this.#assertOpen();
		if (!/^[a-z][a-z0-9._-]{0,127}$/.test(kind))
			throw new TypeError("WAL record kind is invalid");
		const operation = this.#appendQueue.then(() => this.#append(kind, payload));
		this.#appendQueue = operation.then(
			() => undefined,
			() => undefined,
		);
		return operation;
	}

	async #append<T>(kind: string, payload: T): Promise<WalRecord<T>> {
		this.#assertOpen();
		const body: WalBody<T> = {
			version: 1,
			sequence: this.#nextSequence,
			kind,
			payload,
		};
		let bodyJson: string | undefined;
		try {
			bodyJson = JSON.stringify(body);
		} catch {
			throw new TypeError("WAL payload is not JSON serializable");
		}
		if (bodyJson === undefined)
			throw new TypeError("WAL payload is not serializable");
		const length = Buffer.byteLength(bodyJson);
		if (length > this.#maxRecordBytes)
			throw new RangeError("WAL record exceeds the configured limit");
		const normalizedBody = JSON.parse(bodyJson) as WalBody<T>;
		if (!("payload" in normalizedBody))
			throw new TypeError("WAL payload must be JSON serializable");
		const envelope: WalEnvelope<T> = {
			...normalizedBody,
			length,
			sha256: sha256(bodyJson),
		};
		const line = `${JSON.stringify(envelope)}\n`;
		const lineSize = Buffer.byteLength(line);

		if (
			this.#segmentSize > 0 &&
			this.#segmentSize + lineSize > this.#maxSegmentBytes
		) {
			this.#segmentStart = body.sequence;
			this.#segmentSize = 0;
		}

		const handle = await open(
			join(this.#directory, segmentName(this.#segmentStart)),
			"a",
			0o600,
		);
		try {
			await handle.writeFile(line, "utf8");
			await handle.sync();
		} finally {
			await handle.close();
		}

		const record: WalRecord<T> = Object.freeze(normalizedBody);
		this.#records.push(record as WalRecord);
		this.#nextSequence += 1;
		this.#segmentSize += lineSize;
		return structuredClone(record);
	}

	close(): void {
		this.#closed = true;
	}

	#assertOpen(): void {
		if (this.#closed) throw new Error("WAL is closed");
	}
}

async function loadSegment(
	path: string,
	isLast: boolean,
	maxRecordBytes: number,
): Promise<{ records: WalRecord[]; size: number }> {
	const data = await readFile(path);
	const records: WalRecord[] = [];
	let offset = 0;
	let validBytes = 0;

	while (offset < data.length) {
		const newline = data.indexOf(0x0a, offset);
		if (newline < 0) {
			if (!isLast)
				throw new Error(`WAL segment has an incomplete record: ${path}`);
			await quarantineTail(path, data.subarray(offset), validBytes);
			return { records, size: validBytes };
		}
		const line = data.subarray(offset, newline);
		const nextOffset = newline + 1;
		if (line.length === 0) {
			if (!isLast || nextOffset !== data.length)
				throw new Error(`WAL segment contains an empty record: ${path}`);
			validBytes = nextOffset;
			break;
		}

		try {
			records.push(parseEnvelope(line, maxRecordBytes));
		} catch (error) {
			if (!isLast || nextOffset !== data.length) throw error;
			await quarantineTail(path, data.subarray(offset), validBytes);
			return { records, size: validBytes };
		}
		validBytes = nextOffset;
		offset = nextOffset;
	}

	return { records, size: validBytes };
}

function parseEnvelope(line: Buffer, maxRecordBytes: number): WalRecord {
	if (line.length > maxRecordBytes * 2)
		throw new RangeError("WAL envelope exceeds the configured limit");
	const value: unknown = JSON.parse(line.toString("utf8"));
	if (!isObject(value)) throw new TypeError("WAL envelope must be an object");
	if (
		value.version !== 1 ||
		!Number.isSafeInteger(value.sequence) ||
		Number(value.sequence) < 1 ||
		typeof value.kind !== "string"
	) {
		throw new TypeError("WAL envelope metadata is invalid");
	}
	if (
		!Number.isSafeInteger(value.length) ||
		Number(value.length) < 0 ||
		typeof value.sha256 !== "string"
	) {
		throw new TypeError("WAL envelope integrity fields are invalid");
	}
	const body: WalBody<unknown> = {
		version: 1,
		sequence: Number(value.sequence),
		kind: value.kind,
		payload: value.payload,
	};
	const bodyJson = JSON.stringify(body);
	if (
		Buffer.byteLength(bodyJson) !== value.length ||
		Buffer.byteLength(bodyJson) > maxRecordBytes ||
		sha256(bodyJson) !== value.sha256
	) {
		throw new Error("WAL record integrity check failed");
	}
	return Object.freeze(body);
}

async function quarantineTail(
	path: string,
	tail: Buffer,
	validBytes: number,
): Promise<void> {
	if (tail.length === 0) return;
	const quarantine = `${path}.corrupt-${Date.now()}`;
	await writeFile(quarantine, tail, { flag: "wx", mode: 0o600 });
	const handle = await open(path, "r+");
	try {
		await handle.truncate(validBytes);
		await handle.sync();
	} finally {
		await handle.close();
	}
}

async function ensureDirectory(directory: string): Promise<void> {
	await mkdir(dirname(directory), { recursive: true });
	await mkdir(directory, { recursive: true, mode: 0o700 });
	const info = await stat(directory);
	if (!info.isDirectory()) throw new TypeError("WAL path must be a directory");
}

function segmentName(sequence: number): string {
	return `segment-${sequence.toString().padStart(16, "0")}.jsonl`;
}

function sha256(value: string): string {
	return createHash("sha256").update(value).digest("hex");
}

function isObject(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}
