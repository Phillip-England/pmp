package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const projectRegistryFileName = "projects.json"

type registeredProject struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	LastOpened time.Time `json:"last_opened"`
}

func projectRegistryPath() (string, error) {
	root, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, projectRegistryFileName), nil
}

func loadRegisteredProjects() ([]registeredProject, error) {
	path, err := projectRegistryPath()
	if err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(bytes) == 0 {
		return nil, nil
	}
	var projects []registeredProject
	if err := json.Unmarshal(bytes, &projects); err != nil {
		return nil, err
	}
	return normalizeRegisteredProjects(projects), nil
}

func saveRegisteredProjects(projects []registeredProject) error {
	path, err := projectRegistryPath()
	if err != nil {
		return err
	}
	projects = normalizeRegisteredProjects(projects)
	bytes, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func normalizeRegisteredProjects(projects []registeredProject) []registeredProject {
	byPath := map[string]registeredProject{}
	for _, project := range projects {
		cleanPath := strings.TrimSpace(project.Path)
		if cleanPath == "" {
			continue
		}
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			continue
		}
		project.Path = filepath.Clean(absPath)
		if shouldIgnoreProjectPath(project.Path) {
			continue
		}
		project.Name = projectName(project.Path)
		existing, ok := byPath[project.Path]
		if !ok || project.LastOpened.After(existing.LastOpened) {
			byPath[project.Path] = project
		}
	}

	normalized := make([]registeredProject, 0, len(byPath))
	for _, project := range byPath {
		normalized = append(normalized, project)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].LastOpened.Equal(normalized[j].LastOpened) {
			return strings.ToLower(normalized[i].Name) < strings.ToLower(normalized[j].Name)
		}
		return normalized[i].LastOpened.After(normalized[j].LastOpened)
	})
	return normalized
}

func registerProject(root string) error {
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	if shouldIgnoreProjectPath(root) {
		return nil
	}
	projects, err := loadRegisteredProjects()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	found := false
	for i := range projects {
		if filepath.Clean(projects[i].Path) == filepath.Clean(root) {
			projects[i].Name = projectName(root)
			projects[i].LastOpened = now
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, registeredProject{
			Name:       projectName(root),
			Path:       root,
			LastOpened: now,
		})
	}
	return saveRegisteredProjects(projects)
}

func registerCurrentProject() error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	return registerProject(root)
}

func discoverProjects() ([]registeredProject, error) {
	projects, err := loadRegisteredProjects()
	if err != nil {
		return nil, err
	}
	scanned, err := scanForProjects()
	if err != nil {
		return nil, err
	}
	projects = normalizeRegisteredProjects(append(projects, scanned...))
	if err := saveRegisteredProjects(projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func scanForProjects() ([]registeredProject, error) {
	settings, err := loadSystemSettings()
	if err != nil {
		return nil, err
	}
	var projects []registeredProject
	for _, root := range settings.Projects.ScanRoots {
		root := filepath.Clean(root)
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == projectDirName {
				projectRoot := filepath.Dir(path)
				if !shouldIgnoreProjectPath(projectRoot) {
					projects = append(projects, registeredProject{Name: projectName(projectRoot), Path: projectRoot})
				}
				return filepath.SkipDir
			}
			if path != root && shouldSkipProjectScanDir(name) {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return projects, nil
}

func shouldSkipProjectScanDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "Applications", "Library", "Movies", "Music", "Pictures", "node_modules", "vendor":
		return true
	}
	return false
}

func shouldIgnoreProjectPath(root string) bool {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return true
	}

	for _, prefix := range ignoredProjectPathPrefixes() {
		if prefix == "" {
			continue
		}
		if root == prefix || strings.HasPrefix(root, prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func ignoredProjectPathPrefixes() []string {
	seen := map[string]bool{}
	prefixes := []string{
		"/tmp",
		"/var/tmp",
		"/private/tmp",
		"/var/folders",
		"/private/var/folders",
	}

	if tempDir := strings.TrimSpace(os.TempDir()); tempDir != "" {
		prefixes = append(prefixes, tempDir)
		if resolved, err := filepath.EvalSymlinks(tempDir); err == nil && strings.TrimSpace(resolved) != "" {
			prefixes = append(prefixes, resolved)
		}
	}

	cleaned := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = filepath.Clean(strings.TrimSpace(prefix))
		if prefix == "." || prefix == "" || seen[prefix] {
			continue
		}
		seen[prefix] = true
		cleaned = append(cleaned, prefix)
	}
	return cleaned
}

func projectName(root string) string {
	root = filepath.Clean(root)
	name := filepath.Base(root)
	if name == "." || name == "" || name == string(filepath.Separator) {
		return root
	}
	return name
}
