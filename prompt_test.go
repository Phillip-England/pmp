package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDraftRequiresTitleAndBody(t *testing.T) {
	_, err := parseDraft("plain text only")
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title validation error, got %v", err)
	}

	_, err = parseDraft("# Title\n\n")
	if err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestParseDraftExtractsPrompt(t *testing.T) {
	prompt, err := parseDraft("# My Prompt\n\nBody line")
	if err != nil {
		t.Fatalf("parseDraft returned error: %v", err)
	}
	if prompt.Title != "My Prompt" {
		t.Fatalf("unexpected title %q", prompt.Title)
	}
	if !strings.Contains(prompt.Markdown, "Body line") {
		t.Fatalf("expected body in markdown, got %q", prompt.Markdown)
	}
}

func TestParsePromptFields(t *testing.T) {
	prompt, err := parsePromptFields("My Title", "Body text")
	if err != nil {
		t.Fatalf("parsePromptFields returned error: %v", err)
	}
	if prompt.Title != "My Title" {
		t.Fatalf("unexpected title %q", prompt.Title)
	}
	if !strings.Contains(prompt.Markdown, "# My Title") || !strings.Contains(prompt.Markdown, "Body text") {
		t.Fatalf("unexpected markdown %q", prompt.Markdown)
	}
}

func TestFormatAndReadPromptFile(t *testing.T) {
	prompt := Prompt{
		Title:     "Hello",
		Timestamp: time.Date(2026, 5, 8, 12, 30, 0, 0, time.UTC),
		Markdown:  "# Hello\n\nBody.\n",
	}

	formatted := formatPromptFile(prompt)
	if !strings.Contains(formatted, "timestamp: 2026-05-08T12:30:00Z") {
		t.Fatalf("missing timestamp in frontmatter: %q", formatted)
	}
}

func TestBuildPageShowsAllPromptsNewestFirst(t *testing.T) {
	prompts := make([]Prompt, 51)
	for i := range prompts {
		prompts[i] = Prompt{
			Title:     "Prompt",
			Timestamp: time.Date(2026, 5, 8, 12, i, 0, 0, time.UTC),
			Markdown:  "# Prompt\n\nBody\n",
		}
	}

	page, err := buildPage(prompts, map[int]bool{})
	if err != nil {
		t.Fatalf("buildPage returned error: %v", err)
	}
	if len(page.Prompts) != 51 {
		t.Fatalf("expected all prompts on page, got %d", len(page.Prompts))
	}
	if page.Prompts[0].Index != 50 {
		t.Fatalf("expected newest prompt first, got %d", page.Prompts[0].Index)
	}
	if page.Prompts[len(page.Prompts)-1].Index != 0 {
		t.Fatalf("expected oldest prompt last, got %d", page.Prompts[len(page.Prompts)-1].Index)
	}
}

func TestCompilePromptsAll(t *testing.T) {
	prompts := []Prompt{
		{
			Title:     "First",
			Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
			Markdown:  "# First\n\nAlpha\n",
		},
		{
			Title:     "Second",
			Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC),
			Markdown:  "# Second\n\nBeta\n",
		},
	}

	compiled, err := compilePrompts(prompts, nil, nil, nil)
	if err != nil {
		t.Fatalf("compilePrompts returned error: %v", err)
	}
	if !strings.Contains(compiled, "<!-- prompt 0 | 2026-05-08T12:00:00Z -->") {
		t.Fatalf("expected first prompt metadata, got %q", compiled)
	}
	if !strings.Contains(compiled, "# Second\n\nBeta") {
		t.Fatalf("expected second prompt body, got %q", compiled)
	}
}

func TestCompilePromptsRange(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
		{Title: "Two", Timestamp: time.Date(2026, 5, 8, 12, 2, 0, 0, time.UTC), Markdown: "# Two\n\nC\n"},
	}

	compiled, err := compilePrompts(prompts, &compileRange{Start: 1, End: 2}, nil, nil)
	if err != nil {
		t.Fatalf("compilePrompts returned error: %v", err)
	}
	if strings.Contains(compiled, "# Zero") {
		t.Fatalf("did not expect prompt zero in compiled output: %q", compiled)
	}
	if !strings.Contains(compiled, "# One") || !strings.Contains(compiled, "# Two") {
		t.Fatalf("expected selected prompts in compiled output: %q", compiled)
	}
}

func TestCompilePromptsRejectsInvalidRange(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
	}

	_, err := compilePrompts(prompts, &compileRange{Start: 1, End: 1}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected out of bounds error, got %v", err)
	}
}

func TestCompilePromptIndexesSortsSelection(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
		{Title: "Two", Timestamp: time.Date(2026, 5, 8, 12, 2, 0, 0, time.UTC), Markdown: "# Two\n\nC\n"},
	}

	compiled, err := compilePromptIndexes(prompts, []int{2, 0}, nil, nil)
	if err != nil {
		t.Fatalf("compilePromptIndexes returned error: %v", err)
	}
	first := strings.Index(compiled, "# Zero")
	second := strings.Index(compiled, "# Two")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("expected prompts to compile in chronological index order, got %q", compiled)
	}
}

func TestParseCompileArgs(t *testing.T) {
	target, err := parseCompileArgs([]string{"0", "2", "./out.txt"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if target.Range == nil || target.Range.Start != 0 || target.Range.End != 2 {
		t.Fatalf("unexpected compile range: %#v", target.Range)
	}
	if target.OutputFile != "./out.txt" {
		t.Fatalf("unexpected output file %q", target.OutputFile)
	}

	target, err = parseCompileArgs([]string{"./out.txt"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if target.OutputFile != "./out.txt" || target.Range != nil {
		t.Fatalf("unexpected target %#v", target)
	}
}

func TestCompilePromptIndexesIncludesSkills(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
	}

	skills := []Skill{{Name: "ui", Body: "# UI Skill\n\nIntro"}}
	compiled, err := compilePromptIndexes(prompts, []int{0}, skills, []string{"ui"})
	if err != nil {
		t.Fatalf("compilePromptIndexes returned error: %v", err)
	}
	if !strings.HasPrefix(compiled, "# UI Skill\n\nIntro\n\n<!-- prompt 0") {
		t.Fatalf("expected skills at top, got %q", compiled)
	}
}

func TestCompilePromptIndexesIncludesOnlySelectedSkills(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
	}
	skills := []Skill{
		{Name: "ui", Body: "# UI Skill"},
		{Name: "backend", Body: "# Backend Skill"},
	}

	compiled, err := compilePromptIndexes(prompts, []int{0}, skills, []string{"ui"})
	if err != nil {
		t.Fatalf("compilePromptIndexes returned error: %v", err)
	}
	if strings.Contains(compiled, "# Backend Skill") {
		t.Fatalf("did not expect unselected skill in output: %q", compiled)
	}
	if !strings.Contains(compiled, "# UI Skill") {
		t.Fatalf("expected selected skill in output: %q", compiled)
	}
}

func TestParseCompileArgsRejectsInvalidShape(t *testing.T) {
	_, err := parseCompileArgs([]string{"0", "./out.txt"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestParseDeleteArgs(t *testing.T) {
	rng, err := parseDeleteArgs([]string{"2", "4"})
	if err != nil {
		t.Fatalf("parseDeleteArgs returned error: %v", err)
	}
	if rng.Start != 2 || rng.End != 4 {
		t.Fatalf("unexpected delete range %#v", rng)
	}

	rng, err = parseDeleteArgs([]string{"3"})
	if err != nil {
		t.Fatalf("parseDeleteArgs returned error: %v", err)
	}
	if rng.Start != 3 || rng.End != 3 {
		t.Fatalf("unexpected single delete range %#v", rng)
	}
}

func TestReindexMarksAfterDelete(t *testing.T) {
	marks := map[int]bool{1: true, 2: true, 5: true}
	updated := reindexMarksAfterDelete(marks, deleteRange{Start: 2, End: 3})

	if !updated[1] {
		t.Fatalf("expected mark 1 to remain")
	}
	if updated[2] {
		t.Fatalf("did not expect deleted mark to remain")
	}
	if !updated[3] {
		t.Fatalf("expected original mark 5 to shift to 3")
	}
}

func TestParseSelectedPromptIndexesDeduplicatesAndSorts(t *testing.T) {
	indexes, err := parseSelectedPromptIndexes([]string{"3", "1", "3"}, 4)
	if err != nil {
		t.Fatalf("parseSelectedPromptIndexes returned error: %v", err)
	}
	if len(indexes) != 2 || indexes[0] != 1 || indexes[1] != 3 {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestParseSelectedPromptIndexesRequiresSelection(t *testing.T) {
	_, err := parseSelectedPromptIndexes(nil, 4)
	if err == nil || !strings.Contains(err.Error(), "select at least one") {
		t.Fatalf("expected empty selection error, got %v", err)
	}
}

func TestBuildSkillTogglesMarksIncludedSkills(t *testing.T) {
	toggles := buildSkillToggles([]Skill{
		{Name: "ui"},
		{Name: "backend"},
	}, []string{"backend"})
	if len(toggles) != 2 {
		t.Fatalf("expected 2 skill toggles, got %d", len(toggles))
	}
	if toggles[0].Included {
		t.Fatalf("did not expect first skill to be included: %#v", toggles)
	}
	if !toggles[1].Included {
		t.Fatalf("expected second skill to be included: %#v", toggles)
	}
}

func TestBuildPageMarksVisiblePrompt(t *testing.T) {
	prompts := []Prompt{
		{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"},
		{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"},
	}

	page, err := buildPage(prompts, map[int]bool{1: true})
	if err != nil {
		t.Fatalf("buildPage returned error: %v", err)
	}
	if !page.Prompts[0].Marked {
		t.Fatalf("expected newest prompt to be marked")
	}
}

func TestCurrentMarkedIndexReturnsHighestIndex(t *testing.T) {
	index, ok := currentMarkedIndex(map[int]bool{2: true, 5: true})
	if !ok || index != 5 {
		t.Fatalf("expected highest marked index 5, got %d %v", index, ok)
	}
}

func TestIndexesForwardFromMark(t *testing.T) {
	indexes := indexesForwardFromMark(8, map[int]bool{3: true})
	if len(indexes) != 4 {
		t.Fatalf("expected 4 indexes, got %d", len(indexes))
	}
	if indexes[0] != 4 || indexes[len(indexes)-1] != 7 {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestIndexesForwardFromMarkWithoutMark(t *testing.T) {
	indexes := indexesForwardFromMark(8, map[int]bool{})
	if indexes != nil {
		t.Fatalf("expected nil indexes without mark, got %#v", indexes)
	}
}

func TestIndexesForwardFromMarkAtEnd(t *testing.T) {
	indexes := indexesForwardFromMark(5, map[int]bool{4: true})
	if len(indexes) != 0 {
		t.Fatalf("expected no indexes when mark is latest, got %#v", indexes)
	}
}

func TestRunInitCreatesProjectNote(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	notePath := filepath.Join(tempDir, projectNoteFileName)
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(note), "Prompt Memory Project") && !strings.Contains(string(note), "pmp") {
		t.Fatalf("expected project note contents, got %q", string(note))
	}
}

func TestMarkCompiledPromptReplacesExistingMarks(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if err := saveMarks(map[int]bool{1: true, 3: true}); err != nil {
		t.Fatalf("saveMarks returned error: %v", err)
	}
	if err := markCompiledPrompt([]int{2, 4}); err != nil {
		t.Fatalf("markCompiledPrompt returned error: %v", err)
	}

	marks, err := loadMarks()
	if err != nil {
		t.Fatalf("loadMarks returned error: %v", err)
	}
	if len(marks) != 1 || !marks[4] {
		t.Fatalf("expected only latest compiled prompt to remain marked, got %#v", marks)
	}
}

func TestEnsureProjectInitializedCreatesProject(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := ensureProjectInitialized(); err != nil {
		t.Fatalf("ensureProjectInitialized returned error: %v", err)
	}
	if err := ensureProject(); err != nil {
		t.Fatalf("expected initialized project, got %v", err)
	}
}

func TestLoadSkillsCreatesMissingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	path, err := skillsPath()
	if err != nil {
		t.Fatalf("skillsPath returned error: %v", err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("RemoveAll returned error: %v", err)
	}

	skills, err := loadSkills()
	if err != nil {
		t.Fatalf("loadSkills returned error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected no skills, got %#v", skills)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected recreated skills directory, got %v", err)
	}
}

func TestSaveSkillPersistsAndLoads(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if err := saveSkill("ui-guidelines", "# UI\n\nRules"); err != nil {
		t.Fatalf("saveSkill returned error: %v", err)
	}
	skills, err := loadSkills()
	if err != nil {
		t.Fatalf("loadSkills returned error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %#v", skills)
	}
	if skills[0].Name != "ui-guidelines" || !strings.Contains(skills[0].Body, "Rules") {
		t.Fatalf("unexpected skills %#v", skills)
	}
}

func TestLoadSystemSettingsCreatesDefaultFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	configDir := filepath.Join(tempDir, "config")
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if _, err := loadSystemSettings(); err != nil {
		t.Fatalf("loadSystemSettings returned error: %v", err)
	}
	path, err := systemSettingsPath()
	if err != nil {
		t.Fatalf("systemSettingsPath returned error: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	settings, err := loadSystemSettings()
	if err != nil {
		t.Fatalf("loadSystemSettings returned error: %v", err)
	}
	if settings.Theme.AccentColor != "#8fd18a" {
		t.Fatalf("unexpected settings %#v", settings)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected recreated system settings file, got %v", err)
	}
}

func TestSaveThemeSettingsUpdatesSystemSettings(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

	if err := saveThemeSettings(ThemeSettings{AccentColor: "#79c2d0"}); err != nil {
		t.Fatalf("saveThemeSettings returned error: %v", err)
	}

	theme, err := loadThemeSettings()
	if err != nil {
		t.Fatalf("loadThemeSettings returned error: %v", err)
	}
	if theme.AccentColor != "#79c2d0" {
		t.Fatalf("unexpected theme %#v", theme)
	}
}

func TestWriteCompileJSONSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCompileJSON(rec, "hello", nil)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["compiled"] != "hello" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestWriteCompileJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeCompileJSON(rec, "", os.ErrInvalid)

	if rec.Code != 400 {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload["error"] == "" {
		t.Fatalf("expected error payload, got %#v", payload)
	}
}

func TestServeAddressIsConsistent(t *testing.T) {
	if serveAddress != "127.0.0.1:8765" {
		t.Fatalf("unexpected serve address %q", serveAddress)
	}
}

func TestDiscoverProjectsPreservesRegisteredNonTemporaryProjects(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

	projectOne := filepath.Join("/Users", "example", "work", "project-one")
	if err := registerProject(projectOne); err != nil {
		t.Fatalf("registerProject returned error: %v", err)
	}

	projects, err := discoverProjects()
	if err != nil {
		t.Fatalf("discoverProjects returned error: %v", err)
	}

	found := false
	for _, project := range projects {
		if filepath.Clean(project.Path) == filepath.Clean(projectOne) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected retained registered project, got %#v", projects)
	}
}

func TestServeProjectSwitchUpdatesActiveProject(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectA := filepath.Join(t.TempDir(), "alpha")
	projectB := filepath.Join(t.TempDir(), "beta")
	if err := os.MkdirAll(filepath.Join(projectA, projectDirName, promptsDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectB, projectDirName, promptsDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := setProjectRootOverride(projectA); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/projects/switch?path="+url.QueryEscape(projectB), nil)
	serveProjectSwitch(rec, req)

	if rec.Code != 303 {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/projects?switched=1" {
		t.Fatalf("unexpected redirect location %q", location)
	}
	root, err := projectRoot()
	if err != nil {
		t.Fatalf("projectRoot returned error: %v", err)
	}
	if filepath.Clean(root) != filepath.Clean(projectB) {
		t.Fatalf("expected switched project root %q, got %q", projectB, root)
	}
}

func TestRegisterProjectSkipsTemporaryDirectories(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

	tempProject := filepath.Join(os.TempDir(), "TestRunInitCreatesProjectNote2813074405", "001")
	if err := registerProject(tempProject); err != nil {
		t.Fatalf("registerProject returned error: %v", err)
	}

	projects, err := loadRegisteredProjects()
	if err != nil {
		t.Fatalf("loadRegisteredProjects returned error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected temp project to be ignored, got %#v", projects)
	}
}

func TestNormalizeRegisteredProjectsDropsMacTempEntries(t *testing.T) {
	projects := normalizeRegisteredProjects([]registeredProject{
		{
			Name:       "001",
			Path:       "/private/var/folders/sk/841zd5z92vg8tf2zw1__wr4c0000gn/T/TestRunInitCreatesProjectNote2813074405/001",
			LastOpened: time.Date(2026, 5, 9, 15, 49, 39, 0, time.UTC),
		},
		{
			Name:       "real-project",
			Path:       "/Users/example/src/real-project",
			LastOpened: time.Date(2026, 5, 9, 15, 50, 0, 0, time.UTC),
		},
	})

	if len(projects) != 1 {
		t.Fatalf("expected one retained project, got %#v", projects)
	}
	if filepath.Clean(projects[0].Path) != "/Users/example/src/real-project" {
		t.Fatalf("unexpected surviving project %#v", projects[0])
	}
}

func TestNormalizeProjectSettingsDefaultsAndDeduplicatesRoots(t *testing.T) {
	homeDir := filepath.Join(string(filepath.Separator), "Users", "example")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("HOME")
	}()

	settings := normalizeProjectSettings(ProjectSettings{
		ScanRoots: []string{
			filepath.Join(homeDir, "Desktop", "src"),
			filepath.Join(homeDir, "Desktop", "src"),
			filepath.Join(os.TempDir(), "ignored"),
			"",
		},
	})

	if len(settings.ScanRoots) != 1 {
		t.Fatalf("expected one retained scan root, got %#v", settings.ScanRoots)
	}
	if filepath.Clean(settings.ScanRoots[0]) != filepath.Join(homeDir, "Desktop", "src") {
		t.Fatalf("unexpected scan roots %#v", settings.ScanRoots)
	}

	settings = normalizeProjectSettings(ProjectSettings{})
	if len(settings.ScanRoots) == 0 {
		t.Fatalf("expected default project scan roots, got %#v", settings.ScanRoots)
	}
}

func TestDiscoverProjectsScansConfiguredRootsOnly(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

	scanRoot, err := os.MkdirTemp(wd, "scan-root-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(scanRoot)
	}()
	otherRoot, err := os.MkdirTemp(wd, "other-root-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(otherRoot)
	}()

	projectOne := filepath.Join(scanRoot, "project-one")
	projectTwo := filepath.Join(otherRoot, "project-two")
	if err := os.MkdirAll(filepath.Join(projectOne, projectDirName, promptsDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectTwo, projectDirName, promptsDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	settings := defaultSystemSettings()
	settings.Projects = normalizeProjectSettings(ProjectSettings{
		ScanRoots: []string{scanRoot},
	})
	if err := saveSystemSettings(settings); err != nil {
		t.Fatalf("saveSystemSettings returned error: %v", err)
	}

	projects, err := discoverProjects()
	if err != nil {
		t.Fatalf("discoverProjects returned error: %v", err)
	}

	foundOne := false
	foundTwo := false
	for _, project := range projects {
		switch filepath.Clean(project.Path) {
		case filepath.Clean(projectOne):
			foundOne = true
		case filepath.Clean(projectTwo):
			foundTwo = true
		}
	}
	if !foundOne {
		t.Fatalf("expected configured-root project in discovery result, got %#v", projects)
	}
	if foundTwo {
		t.Fatalf("did not expect project outside configured roots, got %#v", projects)
	}
}
