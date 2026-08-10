package update

import "time"

type Candidate struct {
	Version        string    `json:"version"`
	Tag            string    `json:"tag"`
	ReleaseURL     string    `json:"releaseURL"`
	ManifestDigest string    `json:"manifestDigest"`
	BundleURL      string    `json:"bundleURL"`
	BundleDigest   string    `json:"bundleDigest"`
	BundleSize     int64     `json:"bundleSize"`
	Commit         string    `json:"commit"`
	PublishedAt    time.Time `json:"publishedAt"`
}

type Job struct {
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Status struct {
	InstalledVersion string     `json:"installedVersion"`
	Candidate        *Candidate `json:"candidate,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
	Job              *Job       `json:"job,omitempty"`
}

type InstallRequest struct {
	Version        string `json:"version"`
	ManifestDigest string `json:"manifestDigest"`
	Confirmation   string `json:"confirmation"`
	IdempotencyKey string `json:"idempotencyKey"`
	ActorUserID    uint   `json:"actorUserId"`
	ActorUsername  string `json:"actorUsername"`
	RequestID      string `json:"requestId"`
}
