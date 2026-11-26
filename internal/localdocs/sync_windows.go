package localdocs

// Windows has no portable directory fsync; MoveFileEx already replaced the
// file atomically.
func syncDirectory(string) {}
