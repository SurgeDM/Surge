package types

// ObserverOptions requests bounded scalar progress, independently of chunk maps.
// IDs must be full identities. Empty IDs observes all downloads (internal use).
type ObserverOptions struct {
	IDs      []string
	Progress bool
	Chunks   bool
}
