package sandbox

import (
	"os"
	"strings"
)

// DetectContainerEnvironment detects whether the current process is running
// inside a container by checking filesystem markers, environment variables,
// and /proc/1/cgroup contents.
func DetectContainerEnvironment() ContainerEnvironment {
	var proc1Cgroup *string
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		proc1Cgroup = &s
	}

	var envPairs []EnvPair
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envPairs = append(envPairs, EnvPair{Key: parts[0], Value: parts[1]})
		}
	}

	return DetectContainerEnvironmentFrom(SandboxDetectionInputs{
		EnvPairs:           envPairs,
		DockerenvExists:    fileExists("/.dockerenv"),
		ContainerenvExists: fileExists("/run/.containerenv"),
		Proc1Cgroup:        proc1Cgroup,
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
