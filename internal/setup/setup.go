package setup

// FS abstracts filesystem operations for testability.
type FS interface {
	MkdirAll(path string, perm uint32) error
	WriteFile(path string, data []byte, perm uint32) error
	ReadFile(path string) ([]byte, error)
	Stat(path string) (bool, error) // returns (exists, err)
}

// EnsureDirs creates the required project directories.
func EnsureDirs(fs FS, root string) error {
	dirs := []string{"features", "logs", "agent_context"}
	for _, d := range dirs {
		if err := fs.MkdirAll(root+"/"+d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
