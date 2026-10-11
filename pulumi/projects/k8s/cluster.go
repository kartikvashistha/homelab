package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// clusterDir is the per-cluster directory (clusters/<cluster>), holding
// helm-overrides/<release>/values.yaml and manifests/*.yaml. Knowledge of this
// layout lives here, not in the reusable components.
type clusterDir string

func newClusterDir(cluster string) clusterDir {
	return clusterDir(filepath.Join("clusters", cluster))
}

// helmValueFiles returns the override values file for a release, or nil if the
// cluster doesn't override that release.
func (d clusterDir) helmValueFiles(release string) ([]string, error) {
	path := filepath.Join(string(d), "helm-overrides", release, "values.yaml")

	_, err := os.Stat(path)
	switch {
	case err == nil:
		return []string{path}, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	default:
		return nil, fmt.Errorf("read helm values override %s: %w", path, err)
	}
}

// manifests returns the raw manifest files for the cluster. An empty or
// missing directory is valid and yields none.
func (d clusterDir) manifests() ([]string, error) {
	var files []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		matches, err := filepath.Glob(filepath.Join(string(d), "manifests", pattern))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	return files, nil
}
