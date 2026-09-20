//go:build !linux && !darwin

package serve

// On Windows the service identity and access controls are enforced by the
// system ACLs when creating and checking the writable default target.
func verifyDefaultUploadDirectoryOwner(string) error { return nil }
func verifyManagedUploadStateOwner(string) error { return nil }
