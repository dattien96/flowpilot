package runner

type Health struct {
	Status        string `json:"status"`
	RunnerVersion string `json:"runnerVersion"`
	Cwd           string `json:"cwd"`
	Os            string `json:"os"`
	StartedAt     string `json:"startedAt"`
}

type Provider struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version"`
	BinaryPath  string `json:"binaryPath"`
	AuthStatus  string `json:"authStatus"`
	InstallHint string `json:"installHint"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	FilePath    string   `json:"filePath"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type Flow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	FilePath    string   `json:"filePath"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
}
