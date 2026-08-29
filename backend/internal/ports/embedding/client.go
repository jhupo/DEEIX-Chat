// Package embedding defines the external embedding service contract.
package embedding

import "context"

// Client converts text batches into vectors.
type Client interface {
	CallAPI(context.Context, string, string, string, []string, int, int) ([][]float32, error)
}
