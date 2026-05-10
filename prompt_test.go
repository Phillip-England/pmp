package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe returned error: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	return string(output)
}

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
	if !strings.Contains(compiled, "<!-- SKILLS SECTION -->") || !strings.Contains(compiled, "<!-- PROMPTS SECTION -->") {
		t.Fatalf("expected compile sections, got %q", compiled)
	}
	if !strings.Contains(compiled, "<!-- prompt 0 | 2026-05-08T12:00:00Z -->") || !strings.Contains(compiled, "# Second\n\nBeta") {
		t.Fatalf("expected prompt content in output, got %q", compiled)
	}
}

func TestPrefixCompiledWithInstructions(t *testing.T) {
	compiled := prefixCompiledWithInstructions("# Project\n\nFollow this", nil, "<!-- prompt 0 -->\n# Prompt")
	if !strings.Contains(compiled, "<!-- INSTRUCTIONS SECTION -->") || !strings.Contains(compiled, "# Instructions Section") {
		t.Fatalf("expected instructions section, got %q", compiled)
	}
	if !strings.Contains(compiled, "# Project\n\nFollow this") {
		t.Fatalf("expected instructions prefix, got %q", compiled)
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
	target, err := parseCompileArgs([]string{"--range", "0", "2", "--output", "./out.txt", "--skills=ui-guidelines,another-skill", "--update-mark=false"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if target.Range == nil || target.Range.Start != 0 || target.Range.End != 2 {
		t.Fatalf("unexpected compile range: %#v", target.Range)
	}
	if target.UpdateMark {
		t.Fatalf("expected update mark to be disabled")
	}
	if target.OutputFile != "./out.txt" {
		t.Fatalf("unexpected output file %q", target.OutputFile)
	}
	if len(target.IncludedSkills) != 2 || target.IncludedSkills[0] != "ui-guidelines" || target.IncludedSkills[1] != "another-skill" {
		t.Fatalf("unexpected skills %#v", target.IncludedSkills)
	}

	target, err = parseCompileArgs([]string{"--from-mark", "--stdout", "--skill", "ui-guidelines"})
	if err != nil {
		t.Fatalf("parseCompileArgs returned error: %v", err)
	}
	if !target.FromMark || !target.ToStdout || len(target.IncludedSkills) != 1 || target.IncludedSkills[0] != "ui-guidelines" {
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
	if !strings.Contains(compiled, "<!-- SKILLS SECTION -->") || !strings.Contains(compiled, "# UI Skill\n\nIntro") {
		t.Fatalf("expected skills at top, got %q", compiled)
	}
	if !strings.Contains(compiled, "<!-- PROMPTS SECTION -->") || !strings.Contains(compiled, "<!-- prompt 0") {
		t.Fatalf("expected prompts section, got %q", compiled)
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
	_, err := parseCompileArgs([]string{"--range", "0"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got %v", err)
	}

	_, err = parseCompileArgs([]string{"--from-mark", "--range", "0", "1"})
	if err == nil || !strings.Contains(err.Error(), "choose only one compile source") {
		t.Fatalf("expected mutually exclusive source error, got %v", err)
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

func TestRunMarkReplacesExistingMarks(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "Zero",
		Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		Markdown:  "# Zero\n\nA\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "One",
		Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC),
		Markdown:  "# One\n\nB\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}
	if err := saveMarks(map[int]bool{0: true}); err != nil {
		t.Fatalf("saveMarks returned error: %v", err)
	}

	if err := runMark([]string{"1"}); err != nil {
		t.Fatalf("runMark returned error: %v", err)
	}

	marks, err := loadMarks()
	if err != nil {
		t.Fatalf("loadMarks returned error: %v", err)
	}
	if len(marks) != 1 || !marks[1] {
		t.Fatalf("expected only prompt 1 to remain marked, got %#v", marks)
	}
}

func TestRunMarkRejectsMultipleIndexes(t *testing.T) {
	err := runMark([]string{"1", "2"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestIndexesFromMarkExclusive(t *testing.T) {
	indexes, err := indexesFromMarkExclusive(8, map[int]bool{3: true})
	if err != nil {
		t.Fatalf("indexesFromMarkExclusive returned error: %v", err)
	}
	if len(indexes) != 4 {
		t.Fatalf("expected 4 indexes, got %d", len(indexes))
	}
	if indexes[0] != 4 || indexes[len(indexes)-1] != 7 {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestIndexesFromMarkExclusiveWithoutMark(t *testing.T) {
	_, err := indexesFromMarkExclusive(8, map[int]bool{})
	if err == nil || !strings.Contains(err.Error(), "no marked prompt") {
		t.Fatalf("expected missing mark error, got %v", err)
	}
}

func TestIndexesFromMarkExclusiveAtEnd(t *testing.T) {
	indexes, err := indexesFromMarkExclusive(5, map[int]bool{4: true})
	if err != nil {
		t.Fatalf("indexesFromMarkExclusive returned error: %v", err)
	}
	if len(indexes) != 0 {
		t.Fatalf("expected no indexes when mark is latest, got %#v", indexes)
	}
}

func TestRunInitCreatesInstructionsFile(t *testing.T) {
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

	notePath := filepath.Join(tempDir, instructionsFileName)
	note, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(note), ".pmp/responses/") || !strings.Contains(string(note), "Required Response Note") {
		t.Fatalf("expected instructions contents, got %q", string(note))
	}
}

func TestSaveAndLoadProjectInstructions(t *testing.T) {
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
	if err := saveProjectInstructions("# Instructions\n\nBe precise."); err != nil {
		t.Fatalf("saveProjectInstructions returned error: %v", err)
	}
	body, err := loadProjectInstructions()
	if err != nil {
		t.Fatalf("loadProjectInstructions returned error: %v", err)
	}
	if !strings.Contains(body, "Be precise.") {
		t.Fatalf("unexpected instructions body %q", body)
	}
}

func TestRunInitCreatesResponsesDirectory(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(tempDir, projectDirName, responsesDirName)); err != nil {
		t.Fatalf("expected responses directory, got %v", err)
	}
}

func TestCreateProjectAtScanRootInitializesAndActivatesProject(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	scanRoot, err := os.MkdirTemp(wd, "create-project-root-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(scanRoot)
	}()
	if err := createProjectAtScanRoot("alpha", scanRoot, []string{scanRoot}); err != nil {
		t.Fatalf("createProjectAtScanRoot returned error: %v", err)
	}

	target := filepath.Join(scanRoot, "alpha")
	if _, err := os.Stat(filepath.Join(target, projectDirName, promptsDirName)); err != nil {
		t.Fatalf("expected initialized prompts dir, got %v", err)
	}
	root, err := projectRoot()
	if err != nil {
		t.Fatalf("projectRoot returned error: %v", err)
	}
	if filepath.Clean(root) != filepath.Clean(target) {
		t.Fatalf("expected active project root %q, got %q", target, root)
	}
	projects, err := loadRegisteredProjects()
	if err != nil {
		t.Fatalf("loadRegisteredProjects returned error: %v", err)
	}
	if len(projects) != 1 || filepath.Clean(projects[0].Path) != filepath.Clean(target) {
		t.Fatalf("expected created project to be registered, got %#v", projects)
	}
}

func TestCreateProjectAtScanRootRejectsExistingPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	scanRoot, err := os.MkdirTemp(wd, "existing-project-root-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(scanRoot)
	}()

	target := filepath.Join(scanRoot, "alpha")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	err = createProjectAtScanRoot("alpha", scanRoot, []string{scanRoot})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing project error, got %v", err)
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

func TestServeDeletePromptRemovesPrompt(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	first := Prompt{Title: "Zero", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Zero\n\nA\n"}
	second := Prompt{Title: "One", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# One\n\nB\n"}
	if _, err := savePrompt(first); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}
	if _, err := savePrompt(second); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/prompts/delete", strings.NewReader("delete_prompt=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serveDeletePrompt(rec, req)

	if rec.Code != 303 {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/prompts" {
		t.Fatalf("unexpected redirect location %q", location)
	}
	prompts, err := loadPrompts()
	if err != nil {
		t.Fatalf("loadPrompts returned error: %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt after delete, got %d", len(prompts))
	}
	if prompts[0].Title != "Zero" {
		t.Fatalf("expected oldest prompt to remain, got %#v", prompts)
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

func TestRunAddSkillRejectsDuplicateName(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

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
	if err := runAddSkill([]string{"ui-guidelines", "# UI\n\nRules"}); err != nil {
		t.Fatalf("runAddSkill returned error: %v", err)
	}
	err = runAddSkill([]string{"ui-guidelines", "# UI\n\nMore"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestRunAddMemoryAndPrintMemory(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	if err := runAddMemory([]string{"Team Norms", "Prefer short notes."}); err != nil {
		t.Fatalf("runAddMemory returned error: %v", err)
	}
	output := captureStdout(t, func() {
		if err := runPrintMemory([]string{"team-norms"}); err != nil {
			t.Fatalf("runPrintMemory returned error: %v", err)
		}
	})
	if !strings.Contains(output, "# Team Norms") || !strings.Contains(output, "Prefer short notes.") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestRunRemoveMemoryDeletesByNormalizedTitle(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	path, err := saveMemory(Memory{
		Title:     "Decision Log",
		Timestamp: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Body:      "Keep it simple.",
	})
	if err != nil {
		t.Fatalf("saveMemory returned error: %v", err)
	}

	if err := runRemoveMemory([]string{"decision-log"}); err != nil {
		t.Fatalf("runRemoveMemory returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected memory file to be removed, got %v", err)
	}
}

func TestRunPrintPromptByIndex(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "First",
		Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		Markdown:  "# First\n\nAlpha\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runPrintPrompt([]string{"0"}); err != nil {
			t.Fatalf("runPrintPrompt returned error: %v", err)
		}
	})
	if !strings.Contains(output, "# First") || !strings.Contains(output, "Alpha") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestRunListCommandListsSkills(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()

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

	output := captureStdout(t, func() {
		if err := runListCommand([]string{"skills"}); err != nil {
			t.Fatalf("runListCommand returned error: %v", err)
		}
	})
	if !strings.Contains(output, "[ui-guidelines]") {
		t.Fatalf("unexpected output %q", output)
	}
}

func TestRunListPrintsPromptsOldestFirst(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "Older",
		Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		Markdown:  "# Older\n\nFirst\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "Newer",
		Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC),
		Markdown:  "# Newer\n\nSecond\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runList(); err != nil {
			t.Fatalf("runList returned error: %v", err)
		}
	})
	olderIndex := strings.Index(output, "Older")
	newerIndex := strings.Index(output, "Newer")
	if olderIndex == -1 || newerIndex == -1 {
		t.Fatalf("expected both prompts in output, got %q", output)
	}
	if olderIndex > newerIndex {
		t.Fatalf("expected oldest prompt before newest prompt, got %q", output)
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

func TestServeCompilePrefixesProjectInstructions(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}
	if err := saveProjectInstructions("# Instructions\n\nAlways respect project context."); err != nil {
		t.Fatalf("saveProjectInstructions returned error: %v", err)
	}
	if _, err := savePrompt(Prompt{
		Title:     "Zero",
		Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		Markdown:  "# Zero\n\nA\n",
	}); err != nil {
		t.Fatalf("savePrompt returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/compile", strings.NewReader("prompt=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serveCompile(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !strings.Contains(payload["compiled"], "<!-- INSTRUCTIONS SECTION -->") || !strings.Contains(payload["compiled"], "<!-- PROMPTS SECTION -->") {
		t.Fatalf("expected compile sections, got %q", payload["compiled"])
	}
	if !strings.Contains(payload["compiled"], "Always respect project context.") {
		t.Fatalf("expected instructions prefix, got %q", payload["compiled"])
	}
}

func TestLoadResponsesReadsResponseNotes(t *testing.T) {
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
	responseDir := filepath.Join(tempDir, projectDirName, responsesDirName)
	response := Prompt{
		Title:     "Model Response",
		Timestamp: time.Date(2026, 5, 8, 12, 5, 0, 0, time.UTC),
		Markdown:  "# Model Response\n\nImportant result.\n",
	}
	if err := os.WriteFile(filepath.Join(responseDir, "20260508T120500Z-model-response.md"), []byte(formatPromptFile(response)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	responses, err := loadResponses()
	if err != nil {
		t.Fatalf("loadResponses returned error: %v", err)
	}
	if len(responses) != 1 || responses[0].Title != "Model Response" {
		t.Fatalf("unexpected responses %#v", responses)
	}
}

func TestRunRemoveResponseDeletesByIndex(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	responseDir := filepath.Join(projectRoot, projectDirName, responsesDirName)
	response := Prompt{
		Title:     "Model Response",
		Timestamp: time.Date(2026, 5, 8, 12, 5, 0, 0, time.UTC),
		Markdown:  "# Model Response\n\nImportant result.\n",
	}
	path := filepath.Join(responseDir, "20260508T120500Z-model-response.md")
	if err := os.WriteFile(path, []byte(formatPromptFile(response)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := runRemoveResponse([]string{"0"}); err != nil {
		t.Fatalf("runRemoveResponse returned error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected response file to be removed, got %v", err)
	}
}

func TestServeDeleteResponseRemovesResponse(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	responseDir := filepath.Join(projectRoot, projectDirName, responsesDirName)
	first := Prompt{Title: "Older", Timestamp: time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC), Markdown: "# Older\n\nA\n"}
	second := Prompt{Title: "Newer", Timestamp: time.Date(2026, 5, 8, 12, 1, 0, 0, time.UTC), Markdown: "# Newer\n\nB\n"}
	if err := os.WriteFile(filepath.Join(responseDir, "20260508T120000Z-older.md"), []byte(formatPromptFile(first)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(responseDir, "20260508T120100Z-newer.md"), []byte(formatPromptFile(second)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/responses/delete", strings.NewReader("delete_response=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serveDeleteResponse(rec, req)

	if rec.Code != 303 {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/responses" {
		t.Fatalf("unexpected redirect location %q", location)
	}
	responses, err := loadResponses()
	if err != nil {
		t.Fatalf("loadResponses returned error: %v", err)
	}
	if len(responses) != 1 || responses[0].Title != "Older" {
		t.Fatalf("unexpected responses %#v", responses)
	}
}

func TestServeResponsesUsesCardClickWithoutViewButton(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	responseDir := filepath.Join(projectRoot, projectDirName, responsesDirName)
	response := Prompt{
		Title:     "Model Response",
		Timestamp: time.Date(2026, 5, 8, 12, 5, 0, 0, time.UTC),
		Markdown:  "# Model Response\n\nImportant result.\n",
	}
	if err := os.WriteFile(filepath.Join(responseDir, "20260508T120500Z-model-response.md"), []byte(formatPromptFile(response)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/responses", nil)
	serveResponses(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `onclick="openResponseModal('prompt-0')"`) {
		t.Fatalf("expected card click handler in response page, got %q", body)
	}
	if !strings.Contains(body, ".prompt-card.clickable-card:hover") || !strings.Contains(body, "cursor: pointer;") {
		t.Fatalf("expected clickable hover styling in response page, got %q", body)
	}
	if strings.Contains(body, ">View<") {
		t.Fatalf("did not expect view button in response page, got %q", body)
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
	if location := rec.Header().Get("Location"); location != "/new" {
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

func TestServeProjectCreateCreatesProjectUnderConfiguredRoot(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	currentProject := t.TempDir()
	if err := setProjectRootOverride(currentProject); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	scanRoot, err := os.MkdirTemp(wd, "serve-project-create-root-")
	if err != nil {
		t.Fatalf("MkdirTemp returned error: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(scanRoot)
	}()
	settings := defaultSystemSettings()
	settings.Projects = normalizeProjectSettings(ProjectSettings{
		ScanRoots: []string{scanRoot},
	})
	if err := saveSystemSettings(settings); err != nil {
		t.Fatalf("saveSystemSettings returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/projects/create", strings.NewReader(url.Values{
		"project_name": {"bravo"},
		"project_root": {scanRoot},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serveProjectCreate(rec, req)

	if rec.Code != 303 {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/projects?created=1" {
		t.Fatalf("unexpected redirect location %q", location)
	}
	target := filepath.Join(scanRoot, "bravo")
	if _, err := os.Stat(filepath.Join(target, projectDirName, promptsDirName)); err != nil {
		t.Fatalf("expected initialized project at target, got %v", err)
	}
	root, err := projectRoot()
	if err != nil {
		t.Fatalf("projectRoot returned error: %v", err)
	}
	if filepath.Clean(root) != filepath.Clean(target) {
		t.Fatalf("expected active project root %q, got %q", target, root)
	}
}

func newMultipartRequest(t *testing.T, method string, target string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField returned error: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	req := httptest.NewRequest(method, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	serveMemoryAPI(rec, req)
	return rec
}

func TestServeMemoryAPIAcceptsMultipartPost(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	rec := newMultipartRequest(t, "POST", "/memory/api", map[string]string{
		"title": "Decision",
		"body":  "Keep memory submissions multipart-safe.",
	})
	if rec.Code != 201 {
		t.Fatalf("expected created status, got %d with body %q", rec.Code, rec.Body.String())
	}

	memories, err := loadMemories()
	if err != nil {
		t.Fatalf("loadMemories returned error: %v", err)
	}
	if len(memories) != 1 || memories[0].Title != "Decision" {
		t.Fatalf("unexpected memories %#v", memories)
	}
}

func TestServeMemoryAPIAcceptsMultipartPut(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	path, err := saveMemory(Memory{
		Title:     "Initial",
		Timestamp: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Body:      "Before edit",
	})
	if err != nil {
		t.Fatalf("saveMemory returned error: %v", err)
	}

	rec := newMultipartRequest(t, "PUT", "/memory/api", map[string]string{
		"path":  path,
		"title": "Updated",
		"body":  "After edit",
	})
	if rec.Code != 200 {
		t.Fatalf("expected ok status, got %d with body %q", rec.Code, rec.Body.String())
	}

	memories, err := loadMemories()
	if err != nil {
		t.Fatalf("loadMemories returned error: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected one memory after edit, got %#v", memories)
	}
	if memories[0].Title != "Updated" || memories[0].Body != "After edit" {
		t.Fatalf("unexpected memory %#v", memories[0])
	}
}

func TestServeNewPromptMarksFirstPromptWhenUnmarked(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/new", strings.NewReader(url.Values{
		"title": {"First Prompt"},
		"body":  {"This should create the initial mark."},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serveNewPrompt(rec, req)

	if rec.Code != 303 {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}

	marks, err := loadMarks()
	if err != nil {
		t.Fatalf("loadMarks returned error: %v", err)
	}
	if len(marks) != 1 || !marks[0] {
		t.Fatalf("expected first prompt to be marked, got %#v", marks)
	}
}

func TestRunNewCommandSavesPromptAndMarksFirstPrompt(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	if err := runNewCommand([]string{"CLI Title", "CLI Body"}); err != nil {
		t.Fatalf("runNewCommand returned error: %v", err)
	}

	prompts, err := loadPrompts()
	if err != nil {
		t.Fatalf("loadPrompts returned error: %v", err)
	}
	if len(prompts) != 1 || prompts[0].Title != "CLI Title" {
		t.Fatalf("unexpected prompts %#v", prompts)
	}

	marks, err := loadMarks()
	if err != nil {
		t.Fatalf("loadMarks returned error: %v", err)
	}
	if len(marks) != 1 || !marks[0] {
		t.Fatalf("expected first prompt to be marked, got %#v", marks)
	}
}

func TestResolveCompileIndexesCLIFromMark(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("PMP_CONFIG_HOME", configDir); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("PMP_CONFIG_HOME")
	}()
	defer clearProjectRootOverride()

	projectRoot := t.TempDir()
	if err := setProjectRootOverride(projectRoot); err != nil {
		t.Fatalf("setProjectRootOverride returned error: %v", err)
	}
	if err := runInit(); err != nil {
		t.Fatalf("runInit returned error: %v", err)
	}

	_, err := resolveCompileIndexesCLI(compileTarget{FromMark: true}, 5)
	if err == nil || !strings.Contains(err.Error(), "no marked prompt") {
		t.Fatalf("expected missing mark error, got %v", err)
	}
}
