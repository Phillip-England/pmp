package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const serveAddress = "127.0.0.1:8765"

type navItem struct {
	Label   string
	Href    string
	Current bool
}

type currentProjectData struct {
	Name string
	Path string
}

type projectListItem struct {
	Name       string
	Path       string
	LastOpened string
	Current    bool
}

type SkillToggle struct {
	Name     string
	Included bool
}

type newPromptPageData struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Error          string
	Title          string
	Body           string
	Saved          bool
	AccentColor    string
}

type skillsPageData struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Error          string
	Saved          bool
	Skills         []Skill
	NewSkillName   string
	NewSkillBody   string
	AccentColor    string
}

type settingsPageData struct {
	Nav              []navItem
	CurrentProject   currentProjectData
	Error            string
	Saved            bool
	AccentColor      string
	ProjectScanRoots string
}

type instructionsPageData struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Error          string
	Saved          bool
	AccentColor    string
	Body           string
}

type promptListPage struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Prompts        []PromptView
	Skills         []SkillToggle
	Copied         bool
	TotalPrompts   int
	AccentColor    string
	MarkedIndex    int
	HasMark        bool
}

type responseListPage struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Responses      []PromptView
	TotalResponses int
	AccentColor    string
}

type PromptView struct {
	Index      int
	Title      string
	Timestamp  string
	DateValue  string
	SearchText string
	HTMLBody   template.HTML
	ElementID  string
	Marked     bool
	Checked    bool
}

type websocketHub struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

type projectSnapshot struct {
	FileCount int
	LatestMod int64
}

type projectsPageData struct {
	Nav            []navItem
	CurrentProject currentProjectData
	Projects       []projectListItem
	ScanRoots      []string
	AccentColor    string
	Switched       bool
	Created        bool
	Error          string
	NewProjectName string
	NewProjectRoot string
}

func newWebsocketHub() *websocketHub {
	return &websocketHub{conns: map[net.Conn]struct{}{}}
}

func (h *websocketHub) add(conn net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[conn] = struct{}{}
}

func (h *websocketHub) remove(conn net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, conn)
}

func (h *websocketHub) broadcastText(message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.conns {
		if err := writeWebsocketTextFrame(conn, message); err != nil {
			_ = conn.Close()
			delete(h.conns, conn)
		}
	}
}

func runServe() error {
	if err := ensureProject(); err != nil {
		return err
	}
	if err := registerCurrentProject(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", serveAddress)
	if err != nil {
		return err
	}

	hub := newWebsocketHub()
	go watchProjectChanges(hub)

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveHome)
	mux.HandleFunc("/new", serveNewPrompt)
	mux.HandleFunc("/instructions", serveInstructions)
	mux.HandleFunc("/skills", serveSkills)
	mux.HandleFunc("/settings", serveSettings)
	mux.HandleFunc("/prompts", servePrompts)
	mux.HandleFunc("/responses", serveResponses)
	mux.HandleFunc("/prompts/delete", serveDeletePrompt)
	mux.HandleFunc("/projects", serveProjects)
	mux.HandleFunc("/projects/switch", serveProjectSwitch)
	mux.HandleFunc("/projects/create", serveProjectCreate)
	mux.HandleFunc("/compile", serveCompile)
	mux.HandleFunc("/ws", hub.handle)

	url := "http://" + listener.Addr().String()
	_ = openBrowser(url)
	fmt.Println(url)
	return http.Serve(listener, mux)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/new", http.StatusSeeOther)
}

func serveProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := registerCurrentProject(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	systemSettings, err := loadSystemSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentProject, err := loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projects, err := discoverProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := projectsPageData{
		Nav:            buildNav("/projects"),
		CurrentProject: currentProject,
		Projects:       buildProjectListItems(projects, currentProject.Path),
		ScanRoots:      systemSettings.Projects.ScanRoots,
		AccentColor:    themeSettings.AccentColor,
		Switched:       r.URL.Query().Get("switched") == "1",
		Created:        r.URL.Query().Get("created") == "1",
	}
	renderTemplate(w, projectsTemplate, data)
}

func serveProjectSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	if target == "" {
		http.Error(w, "missing project path", http.StatusBadRequest)
		return
	}
	target, err := filepath.Abs(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := ensureProjectAtRoot(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := setProjectRootOverride(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := registerProject(target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/projects?switched=1", http.StatusSeeOther)
}

func serveProjectCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := registerCurrentProject(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	systemSettings, err := loadSystemSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	currentProject, err := loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.Form.Get("project_name"))
	root := strings.TrimSpace(r.Form.Get("project_root"))
	if err := createProjectAtScanRoot(name, root, systemSettings.Projects.ScanRoots); err != nil {
		projects, discoverErr := discoverProjects()
		if discoverErr != nil {
			http.Error(w, discoverErr.Error(), http.StatusInternalServerError)
			return
		}
		data := projectsPageData{
			Nav:            buildNav("/projects"),
			CurrentProject: currentProject,
			Projects:       buildProjectListItems(projects, currentProject.Path),
			ScanRoots:      systemSettings.Projects.ScanRoots,
			AccentColor:    themeSettings.AccentColor,
			Error:          err.Error(),
			NewProjectName: name,
			NewProjectRoot: root,
		}
		renderTemplate(w, projectsTemplate, data)
		return
	}
	http.Redirect(w, r, "/projects?created=1", http.StatusSeeOther)
}

func servePrompts(w http.ResponseWriter, r *http.Request) {
	prompts, marks, err := loadPromptState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	skills, err := loadSkills()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := buildPage(prompts, marks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Nav = buildNav("/prompts")
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Skills = buildSkillToggles(skills, nil)
	data.TotalPrompts = len(prompts)
	data.AccentColor = themeSettings.AccentColor
	data.Copied = r.URL.Query().Get("copied") == "1"
	data.MarkedIndex, data.HasMark = currentMarkedIndex(marks)

	renderTemplate(w, promptsTemplate, data)
}

func serveResponses(w http.ResponseWriter, r *http.Request) {
	responses, err := loadResponses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := buildPromptViews(responses, nil)
	data := responseListPage{
		Nav:            buildNav("/responses"),
		Responses:      views,
		TotalResponses: len(responses),
		AccentColor:    themeSettings.AccentColor,
	}
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderTemplate(w, responsesTemplate, data)
}

func serveCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/prompts", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prompts, _, err := loadPromptState()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	projectInstructions, err := loadProjectInstructions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	skills, err := loadSkills()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeCompileJSON(w, "", err)
		return
	}
	selected, err := resolveCompileIndexes(r.Form, len(prompts))
	if err != nil {
		writeCompileJSON(w, "", err)
		return
	}
	includedSkills := r.Form["include_skill"]
	compiled, err := compilePromptIndexes(prompts, selected, skills, includedSkills)
	if err != nil {
		writeCompileJSON(w, "", err)
		return
	}
	compiled = prefixCompiledWithInstructions(projectInstructions, compiled)
	if shouldUpdateMark(r.Form) && len(selected) > 0 {
		if err := markCompiledPrompt(selected); err != nil {
			writeCompileJSON(w, "", err)
			return
		}
	}
	writeCompileJSON(w, compiled, nil)
}

func serveNewPrompt(w http.ResponseWriter, r *http.Request) {
	systemSettings, err := loadSystemSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := newPromptPageData{
		Nav:         buildNav("/new"),
		AccentColor: systemSettings.Theme.AccentColor,
	}
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, newPromptTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		data.Title = r.Form.Get("title")
		data.Body = r.Form.Get("body")
		prompt, err := parsePromptFields(data.Title, data.Body)
		if err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		if _, err := savePrompt(prompt); err != nil {
			data.Error = err.Error()
			renderTemplate(w, newPromptTemplate, data)
			return
		}
		http.Redirect(w, r, "/new?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveSettings(w http.ResponseWriter, r *http.Request) {
	systemSettings, err := loadSystemSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := settingsPageData{
		Nav:              buildNav("/settings"),
		AccentColor:      systemSettings.Theme.AccentColor,
		ProjectScanRoots: strings.Join(systemSettings.Projects.ScanRoots, "\n"),
	}
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, settingsTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, settingsTemplate, data)
			return
		}
		systemSettings.Theme = normalizeThemeSettings(ThemeSettings{
			AccentColor: r.Form.Get("accent_color"),
		})
		systemSettings.Projects = normalizeProjectSettings(ProjectSettings{
			ScanRoots: strings.Split(r.Form.Get("project_scan_roots"), "\n"),
		})
		data.AccentColor = systemSettings.Theme.AccentColor
		data.ProjectScanRoots = strings.Join(systemSettings.Projects.ScanRoots, "\n")
		if err := saveSystemSettings(systemSettings); err != nil {
			data.Error = err.Error()
			renderTemplate(w, settingsTemplate, data)
			return
		}
		http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveInstructions(w http.ResponseWriter, r *http.Request) {
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := loadProjectInstructions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := instructionsPageData{
		Nav:         buildNav("/instructions"),
		AccentColor: themeSettings.AccentColor,
		Body:        body,
	}
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, instructionsTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, instructionsTemplate, data)
			return
		}
		data.Body = r.Form.Get("body")
		if err := saveProjectInstructions(data.Body); err != nil {
			data.Error = err.Error()
			renderTemplate(w, instructionsTemplate, data)
			return
		}
		http.Redirect(w, r, "/instructions?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveSkills(w http.ResponseWriter, r *http.Request) {
	themeSettings, err := loadThemeSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	skills, err := loadSkills()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := skillsPageData{
		Nav:         buildNav("/skills"),
		AccentColor: themeSettings.AccentColor,
		Skills:      skills,
	}
	data.CurrentProject, err = loadCurrentProjectData()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.URL.Query().Get("saved") == "1" {
		data.Saved = true
	}

	switch r.Method {
	case http.MethodGet:
		renderTemplate(w, skillsTemplate, data)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = err.Error()
			renderTemplate(w, skillsTemplate, data)
			return
		}
		action := r.Form.Get("action")
		switch action {
		case "save":
			name := r.Form.Get("skill_name")
			body := r.Form.Get("skill_body")
			if err := saveSkill(name, body); err != nil {
				data.Error = err.Error()
				data.NewSkillName = name
				data.NewSkillBody = body
				renderTemplate(w, skillsTemplate, data)
				return
			}
		case "delete":
			if err := deleteSkill(r.Form.Get("skill_name")); err != nil {
				data.Error = err.Error()
				renderTemplate(w, skillsTemplate, data)
				return
			}
		default:
			data.Error = "invalid skills action"
			renderTemplate(w, skillsTemplate, data)
			return
		}
		http.Redirect(w, r, "/skills?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func serveDeletePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	indexValue := strings.TrimSpace(r.Form.Get("index"))
	if indexValue == "" {
		indexValue = strings.TrimSpace(r.Form.Get("delete_prompt"))
	}
	index, err := strconv.Atoi(indexValue)
	if err != nil {
		http.Error(w, "invalid prompt index", http.StatusBadRequest)
		return
	}
	if err := runDelete(deleteRange{Start: index, End: index}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/prompts", http.StatusSeeOther)
}

func loadPromptState() ([]Prompt, map[int]bool, error) {
	prompts, err := loadPrompts()
	if err != nil {
		return nil, nil, err
	}
	marks, err := loadMarks()
	if err != nil {
		return nil, nil, err
	}
	return prompts, marks, nil
}

func currentMarkedIndex(marks map[int]bool) (int, bool) {
	if len(marks) == 0 {
		return 0, false
	}

	indexes := make([]int, 0, len(marks))
	for index := range marks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes[len(indexes)-1], true
}

func markCompiledPrompt(selected []int) error {
	if len(selected) == 0 {
		return errors.New("select at least one prompt to compile")
	}

	return saveMarks(map[int]bool{selected[len(selected)-1]: true})
}

func buildNav(current string) []navItem {
	return []navItem{
		{Label: "New", Href: "/new", Current: current == "/new"},
		{Label: "Projects", Href: "/projects", Current: current == "/projects"},
		{Label: "Instructions", Href: "/instructions", Current: current == "/instructions"},
		{Label: "Settings", Href: "/settings", Current: current == "/settings"},
		{Label: "Skills", Href: "/skills", Current: current == "/skills"},
		{Label: "Prompts", Href: "/prompts", Current: current == "/prompts"},
		{Label: "Responses", Href: "/responses", Current: current == "/responses"},
	}
}

func loadCurrentProjectData() (currentProjectData, error) {
	root, err := projectRoot()
	if err != nil {
		return currentProjectData{}, err
	}
	return currentProjectData{
		Name: projectName(root),
		Path: root,
	}, nil
}

func buildProjectListItems(projects []registeredProject, currentPath string) []projectListItem {
	items := make([]projectListItem, 0, len(projects))
	currentPath = filepath.Clean(currentPath)
	for _, project := range projects {
		item := projectListItem{
			Name:    project.Name,
			Path:    project.Path,
			Current: filepath.Clean(project.Path) == currentPath,
		}
		if !project.LastOpened.IsZero() {
			item.LastOpened = project.LastOpened.Local().Format("2006-01-02 15:04:05 MST")
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Current != items[j].Current {
			return items[i].Current
		}
		if items[i].LastOpened == items[j].LastOpened {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if items[i].LastOpened == "" {
			return false
		}
		if items[j].LastOpened == "" {
			return true
		}
		return items[i].LastOpened > items[j].LastOpened
	})
	return items
}

func renderTemplate(w http.ResponseWriter, tpl *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeCompileJSON(w http.ResponseWriter, compiled string, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	payload := map[string]string{}
	if err != nil {
		payload["error"] = err.Error()
		w.WriteHeader(http.StatusBadRequest)
	} else {
		payload["compiled"] = compiled
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func buildPage(prompts []Prompt, marks map[int]bool) (promptListPage, error) {
	data := promptListPage{
		Prompts: buildPromptViews(prompts, marks),
	}
	return data, nil
}

func buildPromptViews(prompts []Prompt, marks map[int]bool) []PromptView {
	views := make([]PromptView, 0, len(prompts))
	for index := len(prompts) - 1; index >= 0; index-- {
		prompt := prompts[index]
		marked := false
		if marks != nil {
			marked = marks[index]
		}
		views = append(views, PromptView{
			Index:      index,
			Title:      prompt.Title,
			Timestamp:  prompt.Timestamp.Local().Format("2006-01-02 15:04:05 MST"),
			DateValue:  prompt.Timestamp.Local().Format("2006-01-02"),
			SearchText: strings.ToLower(prompt.Title + "\n" + prompt.Markdown),
			HTMLBody:   template.HTML(renderMarkdown(prompt.Markdown)),
			ElementID:  fmt.Sprintf("prompt-%d", index),
			Marked:     marked,
			Checked:    false,
		})
	}
	return views
}

func buildSkillToggles(skills []Skill, included []string) []SkillToggle {
	includedSet := skillNamesSet(included)
	toggles := make([]SkillToggle, 0, len(skills))
	for _, skill := range skills {
		toggles = append(toggles, SkillToggle{
			Name:     skill.Name,
			Included: includedSet[normalizeSkillName(skill.Name)],
		})
	}
	return toggles
}

func indexesForwardFromMark(promptCount int, marks map[int]bool) []int {
	markedIndex, ok := currentMarkedIndex(marks)
	if !ok {
		return nil
	}

	if markedIndex+1 >= promptCount {
		return []int{}
	}

	indexes := make([]int, 0, promptCount-markedIndex-1)
	for i := markedIndex + 1; i < promptCount; i++ {
		indexes = append(indexes, i)
	}
	return indexes
}

func indexesFromMarkInclusive(promptCount int, marks map[int]bool) ([]int, error) {
	markedIndex, ok := currentMarkedIndex(marks)
	if !ok {
		return nil, errors.New("no marked prompt available")
	}
	if markedIndex < 0 || markedIndex >= promptCount {
		return nil, fmt.Errorf("marked prompt index %d is out of bounds; highest prompt index is %d", markedIndex, promptCount-1)
	}
	indexes := make([]int, 0, promptCount-markedIndex)
	for i := markedIndex; i < promptCount; i++ {
		indexes = append(indexes, i)
	}
	return indexes, nil
}

func parseSelectedPromptIndexes(values []string, promptCount int) ([]int, error) {
	if len(values) == 0 {
		return nil, errors.New("select at least one prompt to compile")
	}

	selected := make([]int, 0, len(values))
	seen := map[int]bool{}
	for _, value := range values {
		index, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid prompt index %q", value)
		}
		if index < 0 || index >= promptCount {
			return nil, fmt.Errorf("prompt index %d is out of bounds; highest prompt index is %d", index, promptCount-1)
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		selected = append(selected, index)
	}

	sort.Ints(selected)
	return selected, nil
}

func resolveCompileIndexes(values url.Values, promptCount int) ([]int, error) {
	mode := strings.TrimSpace(values.Get("mode"))
	switch mode {
	case "", "selected":
		return parseSelectedPromptIndexes(values["prompt"], promptCount)
	case "all":
		indexes := make([]int, 0, promptCount)
		for i := 0; i < promptCount; i++ {
			indexes = append(indexes, i)
		}
		return indexes, nil
	case "from_mark":
		marks, err := loadMarks()
		if err != nil {
			return nil, err
		}
		return indexesFromMarkInclusive(promptCount, marks)
	case "range":
		start, err := strconv.Atoi(strings.TrimSpace(values.Get("range_start")))
		if err != nil {
			return nil, errors.New("range start must be a valid prompt index")
		}
		end, err := strconv.Atoi(strings.TrimSpace(values.Get("range_end")))
		if err != nil {
			return nil, errors.New("range end must be a valid prompt index")
		}
		return compileIndexesForRange(promptCount, compileRange{Start: start, End: end})
	default:
		return nil, errors.New("invalid compile mode")
	}
}

func compileIndexesForRange(promptCount int, rng compileRange) ([]int, error) {
	if rng.Start < 0 || rng.End < 0 {
		return nil, errors.New("compile range indexes must be non-negative")
	}
	if rng.Start > rng.End {
		return nil, errors.New("compile start index must be less than or equal to end index")
	}
	if promptCount == 0 {
		return []int{}, nil
	}
	if rng.End >= promptCount {
		return nil, fmt.Errorf("compile range out of bounds; highest prompt index is %d", promptCount-1)
	}
	indexes := make([]int, 0, rng.End-rng.Start+1)
	for i := rng.Start; i <= rng.End; i++ {
		indexes = append(indexes, i)
	}
	return indexes, nil
}

func shouldUpdateMark(values url.Values) bool {
	raw := strings.TrimSpace(strings.ToLower(values.Get("update_mark")))
	return raw == "1" || raw == "true" || raw == "on" || raw == "yes"
}

func renderMarkdown(markdown string) string {
	var buf bytes.Buffer
	lines := splitLines(markdown)
	inParagraph := false
	inPre := false

	closeParagraph := func() {
		if inParagraph {
			buf.WriteString("</p>")
			inParagraph = false
		}
	}

	for _, line := range lines {
		trimmed := line
		if trimmed == "```" {
			closeParagraph()
			if inPre {
				buf.WriteString("</code></pre>")
			} else {
				buf.WriteString("<pre><code>")
			}
			inPre = !inPre
			continue
		}
		if inPre {
			template.HTMLEscape(&buf, []byte(line))
			buf.WriteByte('\n')
			continue
		}
		if trimmed == "" {
			closeParagraph()
			continue
		}
		if len(trimmed) > 2 && trimmed[:2] == "# " {
			closeParagraph()
			buf.WriteString("<h1>")
			template.HTMLEscape(&buf, []byte(trimmed[2:]))
			buf.WriteString("</h1>")
			continue
		}
		if !inParagraph {
			buf.WriteString("<p>")
			inParagraph = true
		} else {
			buf.WriteByte(' ')
		}
		template.HTMLEscape(&buf, []byte(trimmed))
	}

	closeParagraph()
	if inPre {
		buf.WriteString("</code></pre>")
	}
	return buf.String()
}

func splitLines(s string) []string {
	lines := make([]string, 0)
	current := bytes.Buffer{}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current.String())
			current.Reset()
			continue
		}
		if s[i] != '\r' {
			current.WriteByte(s[i])
		}
	}
	lines = append(lines, current.String())
	return lines
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func watchProjectChanges(hub *websocketHub) {
	previous, err := snapshotProject()
	if err != nil {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		current, err := snapshotProject()
		if err != nil {
			continue
		}
		if current != previous {
			previous = current
			hub.broadcastText("reload")
		}
	}
}

func snapshotProject() (projectSnapshot, error) {
	_, promptsDir, _, err := projectPaths()
	if err != nil {
		return projectSnapshot{}, err
	}

	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return projectSnapshot{}, err
	}

	snapshot := projectSnapshot{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return projectSnapshot{}, err
		}
		snapshot.FileCount++
		mod := info.ModTime().UnixNano()
		if mod > snapshot.LatestMod {
			snapshot.LatestMod = mod
		}
	}

	marks, err := marksPath()
	if err != nil {
		return projectSnapshot{}, err
	}
	if info, err := os.Stat(marks); err == nil {
		mod := info.ModTime().UnixNano()
		if mod > snapshot.LatestMod {
			snapshot.LatestMod = mod
		}
	}

	return snapshot, nil
}

func (h *websocketHub) handle(w http.ResponseWriter, r *http.Request) {
	if !headerContainsToken(r.Header, "Connection", "Upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}

	accept := websocketAccept(key)
	if _, err := rw.WriteString(
		"HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n\r\n",
	); err != nil {
		_ = conn.Close()
		return
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	h.add(conn)
	go h.readLoop(conn)
}

func (h *websocketHub) readLoop(conn net.Conn) {
	defer func() {
		h.remove(conn)
		_ = conn.Close()
	}()

	buffer := make([]byte, 4096)
	for {
		if _, err := conn.Read(buffer); err != nil {
			return
		}
	}
}

func headerContainsToken(header http.Header, key string, token string) bool {
	for _, value := range header.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeWebsocketTextFrame(conn net.Conn, message string) error {
	payload := []byte(message)
	frame := []byte{0x81}

	switch {
	case len(payload) < 126:
		frame = append(frame, byte(len(payload)))
	case len(payload) <= 65535:
		frame = append(frame, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		return errors.New("websocket payload too large")
	}

	frame = append(frame, payload...)
	_, err := conn.Write(frame)
	return err
}

const baseStyles = `
  :root {
    color-scheme: dark;
    --bg: #000000;
    --panel: #090909;
    --panel-strong: #111111;
    --border: #2f2f2f;
    --text: #ffffff;
    --muted: #a6a6a6;
    --accent: #ffffff;
    --action: #8fd18a;
    --mark-bg: #1a1a1a;
    --mark-border: #ffffff;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    background:
      radial-gradient(circle at top, rgba(143, 209, 138, 0.12), transparent 28rem),
      linear-gradient(180deg, #050505 0%, #000000 100%);
    color: var(--text);
    font: 14px/1.45 "Symbols Nerd Font Mono", "SauceCodePro Nerd Font Mono", "CaskaydiaMono Nerd Font", "JetBrainsMono Nerd Font", ui-monospace, "SFMono-Regular", "Cascadia Mono", "Cascadia Code", Menlo, Consolas, monospace;
    letter-spacing: 0.01em;
  }
  a {
    color: var(--text);
    text-decoration: none;
  }
  .shell {
    width: min(72rem, calc(100% - 1rem));
    margin: 0 auto;
    padding: 0.75rem 0 1rem;
  }
  .nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-bottom: 0.75rem;
    text-transform: uppercase;
  }
  .nav-links {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-left: auto;
  }
  .nav-brand {
    margin-right: 0.35rem;
    padding: 0.1rem 0.15rem;
    color: var(--action);
    font-family: "Cascadia Code", "Cascadia Mono", "SFMono-Regular", Menlo, Consolas, monospace;
    font-size: 1.3rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    line-height: 1;
    text-transform: lowercase;
    text-shadow: 0 0 1.4rem color-mix(in srgb, var(--action) 32%, transparent);
  }
  .nav a, .button, button {
    border: 1px solid var(--border);
    background: var(--panel-strong);
    color: var(--text);
    border-radius: 0.55rem;
    padding: 0.45rem 0.7rem;
    font: inherit;
    cursor: pointer;
    transition: transform 0.12s ease, border-color 0.12s ease, background-color 0.12s ease, box-shadow 0.12s ease;
  }
  .nav a:hover, .button:hover, button:hover {
    border-color: var(--action);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--action) 40%, transparent);
    transform: translateY(-1px);
  }
  .nav a.current, .button.primary, button.primary {
    background: var(--action);
    color: #000000;
    border-color: var(--action);
  }
  .nav a.current:hover, .button.primary:hover, button.primary:hover {
    background: color-mix(in srgb, var(--action) 88%, white);
    border-color: color-mix(in srgb, var(--action) 88%, white);
  }
  .panel, details {
    border: 1px solid var(--border);
    background: var(--panel);
    border-radius: 0.8rem;
  }
  .panel {
    padding: 0.8rem 0.9rem;
    box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.02), 0 1rem 2.5rem rgba(0, 0, 0, 0.22);
  }
  .panel + .panel, .stack, .pager {
    margin-top: 0.75rem;
  }
  .compact {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }
  .project-strip {
    margin-bottom: 0.75rem;
    align-items: flex-start;
  }
  .project-name {
    font-size: 1rem;
    text-transform: uppercase;
  }
  .project-path {
    word-break: break-word;
  }
  .muted {
    color: var(--muted);
  }
  .actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  .actions.spaced {
    margin-bottom: 0.8rem;
  }
  .icon-button {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
  }
  .icon-button svg {
    width: 0.95rem;
    height: 0.95rem;
    display: block;
    fill: currentColor;
  }
  .icon-button span {
    display: none;
  }
  .stack {
    display: grid;
    gap: 0.55rem;
  }
  .create-project-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(12rem, 1fr) auto;
    gap: 0.85rem;
    align-items: end;
  }
  .filter-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.7fr) repeat(2, minmax(10rem, 1fr)) auto;
    gap: 0.7rem;
    align-items: end;
  }
  .prompt-filters {
    padding: 1rem 1.1rem 1.05rem;
  }
  .prompt-filters .filter-grid {
    gap: 0.95rem;
  }
  .prompt-filters .actions {
    align-self: end;
    padding-bottom: 0.05rem;
  }
  .prompt-filters .small {
    display: block;
    margin-top: 0.8rem;
  }
  .prompt-filters input[type="search"],
  .prompt-filters input[type="date"] {
    padding: 0.8rem 0.9rem;
  }
  .prompt-grid, .prompt-picker {
    display: flex;
    flex-wrap: wrap;
    gap: 0.7rem;
    align-items: flex-start;
  }
  details {
    padding: 0.7rem 0.85rem;
    flex: 0 1 19rem;
    min-width: 16rem;
  }
  details[open] {
    flex-basis: 100%;
  }
  details.marked, .prompt-option.marked {
    border-color: var(--mark-border);
    background: var(--mark-bg);
  }
  summary {
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .summary-row {
    display: grid;
    gap: 0.2rem;
  }
  .summary-title {
    font-size: 0.98rem;
  }
  .summary-meta, .small {
    color: var(--muted);
    font-size: 0.76rem;
  }
  .mark-badge {
    display: inline-block;
    margin-left: 0.45rem;
    padding: 0.05rem 0.35rem;
    border: 1px solid #ffffff;
    border-radius: 999px;
    font-size: 0.72rem;
    line-height: 1.3;
    vertical-align: middle;
  }
  article {
    margin-top: 0.7rem;
    padding-top: 0.7rem;
    border-top: 1px solid var(--border);
  }
  article p, article h1 {
    margin-top: 0;
  }
  .pager {
    display: flex;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .prompt-option {
    display: flex;
    gap: 0.55rem;
    align-items: start;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 0.7rem;
    background: #050505;
    cursor: pointer;
    transition: border-color 0.12s ease, transform 0.12s ease, box-shadow 0.12s ease;
    flex: 0 1 18rem;
    min-width: 15rem;
  }
  .prompt-option:hover {
    border-color: var(--action);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--action) 40%, transparent);
    transform: translateY(-1px);
  }
  .prompt-option input {
    margin-top: 0.15rem;
  }
  .error {
    color: #ffb8b8;
    border-color: #7a2f2f;
    background: #160909;
  }
  .success {
    color: #d6ffd3;
    border-color: var(--action);
    background: #091309;
  }
  [hidden] {
    display: none !important;
  }
  textarea, input[type="text"], input[type="search"], input[type="date"], input[type="number"], input[type="color"], select {
    width: 100%;
    border: 1px solid var(--border);
    background: #050505;
    color: var(--text);
    border-radius: 0.7rem;
    padding: 0.7rem 0.8rem;
    font: inherit;
  }
  textarea {
    min-height: 18rem;
    resize: vertical;
  }
  input[type="color"] {
    min-height: 3rem;
    padding: 0.3rem;
  }
  .form-grid {
    display: grid;
    gap: 0.7rem;
  }
  .compile-controls {
    display: grid;
    gap: 0.8rem;
  }
  .compile-controls-row {
    display: flex;
    flex-wrap: wrap;
    gap: 0.8rem;
    align-items: flex-end;
  }
  .compile-range-field {
    flex: 0 1 8rem;
    min-width: 7rem;
  }
  .compile-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 2.75rem;
  }
  .compile-toggle input {
    margin: 0;
  }
  .label {
    display: grid;
    gap: 0.35rem;
  }
  .label-row {
    display: flex;
    align-items: center;
    gap: 0.45rem;
  }
  .dot {
    width: 0.65rem;
    height: 0.65rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: #050505;
    display: inline-block;
    transition: background-color 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
  }
  .dot.waiting {
    background: #e0b84d;
    border-color: #e0b84d;
    box-shadow: 0 0 0.45rem rgba(224, 184, 77, 0.65);
  }
  .dot.live {
    background: #d93025;
    border-color: #d93025;
    box-shadow: 0 0 0.45rem rgba(217, 48, 37, 0.7);
  }
  .dot.active {
    background: var(--action);
    border-color: var(--action);
    box-shadow: 0 0 0.45rem rgba(143, 209, 138, 0.6);
  }
  .dot.heard {
    transform: scale(1.18);
  }
  .instructions {
    color: var(--muted);
    font-size: 0.88rem;
    line-height: 1.35;
  }
  .section-title {
    margin: 0 0 0.2rem;
    font-size: 0.95rem;
  }
  .prompt-actions {
    display: flex;
    justify-content: flex-end;
    margin-top: 0.7rem;
  }
  .project-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    align-items: stretch;
  }
  .project-card {
    display: grid;
    gap: 0.7rem;
    flex: 0 1 17rem;
    min-width: 14rem;
    padding: 0.8rem;
    align-content: space-between;
  }
  .project-meta {
    display: grid;
    gap: 0.2rem;
  }
  .skill-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
  }
  .skill-card {
    display: grid;
    gap: 0.7rem;
    padding: 0.8rem;
    border: 1px solid var(--border);
    border-radius: 0.8rem;
    background: var(--panel);
    flex: 0 1 16rem;
    min-width: 13rem;
  }
  .skill-card-title {
    font-weight: 600;
    word-break: break-word;
  }
  .prompt-option-copy {
    display: grid;
    gap: 0.18rem;
  }
  .prompt-card {
    display: grid;
    gap: 0.7rem;
    padding: 0.8rem;
    border: 1px solid var(--border);
    border-radius: 0.8rem;
    background: var(--panel);
    flex: 0 1 18rem;
    min-width: 15rem;
  }
  .prompt-card.marked {
    border-color: var(--mark-border);
    background: var(--mark-bg);
  }
  .prompt-card-header {
    display: flex;
    align-items: flex-start;
    gap: 0.6rem;
  }
  .prompt-card-copy {
    display: grid;
    gap: 0.18rem;
  }
  .skill-toggles {
    display: flex;
    flex-wrap: wrap;
    gap: 0.55rem;
  }
  .skill-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: #050505;
  }
  dialog.prompt-modal {
    width: min(56rem, calc(100% - 1.5rem));
    border: 1px solid var(--border);
    border-radius: 0.9rem;
    background: var(--panel);
    color: var(--text);
    padding: 0;
  }
  dialog.prompt-modal::backdrop {
    background: rgba(0, 0, 0, 0.65);
  }
  .modal-shell {
    padding: 0.9rem 1rem 1rem;
  }
  .modal-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 0.8rem;
    margin-bottom: 0.8rem;
  }
  .modal-body {
    max-height: 70vh;
    overflow: auto;
    padding-top: 0.8rem;
    border-top: 1px solid var(--border);
  }
  mark.search-hit {
    background: #f3dd77;
    color: #111111;
    padding: 0 0.15rem;
    border-radius: 0.2rem;
  }
  .modal-actions {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    margin-top: 0.9rem;
    flex-wrap: wrap;
  }
  .danger {
    border-color: #7a2f2f;
    color: #ffb8b8;
  }
  .clipboard-source {
    position: absolute;
    left: -9999px;
    width: 1px;
    height: 1px;
    opacity: 0;
  }
  @media (max-width: 640px) {
    body { font-size: 13px; }
    .compact {
      flex-direction: column;
      align-items: flex-start;
    }
    .nav {
      align-items: flex-start;
    }
    .nav-links {
      width: 100%;
      justify-content: flex-start;
      margin-left: 0;
    }
    .filter-grid {
      grid-template-columns: 1fr;
    }
    .create-project-grid {
      grid-template-columns: 1fr;
    }
    details, .prompt-option, .prompt-card, .skill-card, .project-card {
      min-width: 100%;
      flex-basis: 100%;
    }
  }
`

const liveReloadScript = `<script>
  (function() {
    var scheme = window.location.protocol === "https:" ? "wss://" : "ws://";
    var socket = new WebSocket(scheme + window.location.host + "/ws");
    socket.onmessage = function(event) {
      if (event.data === "reload") {
        window.location.reload();
      }
    };
  })();
</script>`

const currentProjectBanner = `
    <section class="panel project-strip">
      <div class="small">Working directory: <span class="project-name">{{.CurrentProject.Name}}</span></div>
    </section>`

const navBrand = `<div class="nav-brand">pmp</div>`

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    <section class="panel compact">
      <div>{{.TotalPrompts}} prompts{{if .HasMark}} <span class="muted">marked: {{.MarkedIndex}}</span>{{end}}</div>
      <div class="actions">
        <a class="button" href="/new">New</a>
        <a class="button" href="/prompts">Browse</a>
        <a class="button primary" href="/prompts">Prompts</a>
      </div>
    </section>
  </main>
  ` + liveReloadScript + `
</body>
</html>`))

var newPromptTemplate = template.Must(template.New("new-prompt").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp new</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
  ` + currentProjectBanner + `
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel success">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <label class="label">
          <span>Title</span>
          <input id="new-prompt-title" type="text" name="title" value="{{.Title}}" autofocus>
        </label>
        <label class="label">
          <span>Body</span>
          <textarea name="body">{{.Body}}</textarea>
        </label>
        <div class="actions spaced">
          <button type="submit" class="primary">Save prompt</button>
        </div>
      </form>
    </section>
  </main>
  <script>
    (function() {
      var input = document.getElementById('new-prompt-title');
      if (!input) {
        return;
      }
      input.focus();
      input.select();
    })();
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var skillsTemplate = template.Must(template.New("skills").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp skills</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel success">saved</section>{{end}}
    <section class="panel stack">
      <form method="post" class="form-grid">
        <label class="label">
          <span>New skill name</span>
          <input type="text" name="skill_name" value="{{.NewSkillName}}">
        </label>
        <label class="label">
          <span>Skill body</span>
          <textarea name="skill_body">{{.NewSkillBody}}</textarea>
        </label>
        <input type="hidden" name="action" value="save">
        <div class="actions spaced">
          <button type="submit" class="primary">Save skill</button>
        </div>
      </form>
      {{if .Skills}}
      <div class="skill-grid">
        {{range .Skills}}
        <section class="skill-card">
          <div class="skill-card-title">{{.Name}}</div>
          <textarea class="clipboard-source" readonly>{{.Body}}</textarea>
          <div class="actions">
            <button type="button" onclick="openSkillModal(this)">View skill</button>
          </div>
          <dialog class="prompt-modal">
            <div class="modal-shell">
              <div class="modal-header">
                <div>
                  <div class="summary-title">{{.Name}}</div>
                  <div class="summary-meta">system-wide skill</div>
                </div>
                <button type="button" onclick="closeSkillModal(this)">Close</button>
              </div>
              <div class="modal-body">
                <form method="post" class="form-grid">
                  <input type="hidden" name="action" value="save">
                  <input type="hidden" name="skill_name" value="{{.Name}}">
                  <label class="label">
                    <span>Body</span>
                    <textarea name="skill_body">{{.Body}}</textarea>
                  </label>
                  <div class="modal-actions">
                    <button type="button" onclick="copySkillBody(this)">Copy skill</button>
                    <button type="submit" class="primary">Update skill</button>
                  </div>
                </form>
                <form method="post" class="modal-actions">
                  <input type="hidden" name="action" value="delete">
                  <input type="hidden" name="skill_name" value="{{.Name}}">
                  <button type="submit" class="danger">Delete skill</button>
                </form>
              </div>
            </div>
          </dialog>
        </section>
        {{end}}
      </div>
      {{else}}
      <div class="muted">No skills yet.</div>
      {{end}}
    </section>
  </main>
  <script>
    function closestSkillDialog(node) {
      var card = node ? node.closest('.skill-card') : null;
      return card ? card.querySelector('dialog') : null;
    }
    function openSkillModal(button) {
      var dialog = closestSkillDialog(button);
      if (!dialog) {
        return;
      }
      if (typeof dialog.showModal === 'function') {
        dialog.showModal();
      } else {
        dialog.setAttribute('open', 'open');
      }
    }
    function closeSkillModal(button) {
      var dialog = button ? button.closest('dialog') : null;
      if (!dialog) {
        return;
      }
      if (typeof dialog.close === 'function') {
        dialog.close();
      } else {
        dialog.removeAttribute('open');
      }
    }
    function copySkillBody(button) {
      var card = button ? button.closest('.skill-card') : null;
      var source = card ? card.querySelector('.clipboard-source') : null;
      if (!source) {
        return;
      }
      var text = source.value || source.textContent || '';
      function fallbackCopy() {
        source.focus();
        source.select();
        try {
          document.execCommand('copy');
        } catch (err) {
        }
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).catch(fallbackCopy);
      } else {
        fallbackCopy();
      }
    }
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var settingsTemplate = template.Must(template.New("settings").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp settings</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel success">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <div class="stack">
          <h2 class="section-title">Theme</h2>
          <div class="instructions">Accent color is also system-wide.</div>
        </div>
        <label class="label">
          <span>Accent color</span>
          <input type="color" name="accent_color" value="{{.AccentColor}}">
        </label>
        <div class="stack">
          <h2 class="section-title">Project Scan Roots</h2>
          <div class="instructions">PMP only scans these directories for projects. Keep this list small and focused.</div>
          <div class="instructions">Enter one absolute path per line. Projects you open directly are still kept in the local registry even if they live outside these roots.</div>
        </div>
        <label class="label">
          <span>Directories to scan</span>
          <textarea name="project_scan_roots">{{.ProjectScanRoots}}</textarea>
        </label>
	        <div class="actions spaced">
	          <button type="submit" class="primary">Save settings</button>
	        </div>
	      </form>
	    </section>
	  </main>
  <script>
    (function() {
      var input = document.querySelector('input[name="accent_color"]');
      if (!input) {
        return;
      }
      input.addEventListener('input', function(event) {
        document.documentElement.style.setProperty('--action', event.currentTarget.value);
      });
    })();
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var instructionsTemplate = template.Must(template.New("instructions").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp instructions</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    {{if .Saved}}<section class="panel success">saved</section>{{end}}
    <section class="panel">
      <form method="post" class="form-grid">
        <div class="stack">
          <h2 class="section-title">Instructions</h2>
          <div class="instructions">This text is stored in <code>INSTRUCTIONS.md</code> for the current project.</div>
          <div class="instructions">Keep it generic. It should explain how to use the compiled content, what the sections mean, and how response notes must be written back into <code>.pmp/responses/</code>.</div>
          <div class="instructions">It is automatically prefixed onto every compilation before selected skills and prompts.</div>
        </div>
        <label class="label">
          <span>Instruction text</span>
          <textarea name="body">{{.Body}}</textarea>
        </label>
        <div class="actions spaced">
          <button type="submit" class="primary">Save instructions</button>
        </div>
      </form>
    </section>
  </main>
  ` + liveReloadScript + `
</body>
</html>`))

var responsesTemplate = template.Must(template.New("responses").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp responses</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    <section class="panel compact">
      <div>{{.TotalResponses}} responses</div>
      <div class="instructions">Response notes are markdown files read from <code>.pmp/responses/</code>.</div>
    </section>
    <section class="panel stack">
      <div class="prompt-grid">
        {{range .Responses}}
        <section class="prompt-card" data-date="{{.DateValue}}">
          <div class="prompt-card-copy">
            <strong>{{.Title}}</strong>
            <span class="small">{{.Timestamp}}</span>
          </div>
          <div class="actions">
            <button type="button" onclick="openResponseModal('{{.ElementID}}')">View</button>
          </div>
          <div id="{{.ElementID}}-content" hidden>{{.HTMLBody}}</div>
          <dialog id="{{.ElementID}}" class="prompt-modal">
            <div class="modal-shell">
              <div class="modal-header">
                <div>
                  <div class="summary-title">{{.Title}}</div>
                  <div class="summary-meta">{{.Timestamp}}</div>
                </div>
                <button type="button" onclick="closeResponseModal('{{.ElementID}}')">Close</button>
              </div>
              <div class="modal-body" id="{{.ElementID}}-body"></div>
            </div>
          </dialog>
        </section>
        {{else}}
        <section class="panel">
          <div class="muted">No responses yet.</div>
        </section>
        {{end}}
      </div>
    </section>
  </main>
  <script>
    function openResponseModal(id) {
      var dialog = document.getElementById(id);
      var body = document.getElementById(id + '-body');
      var content = document.getElementById(id + '-content');
      if (!dialog || !body || !content) {
        return;
      }
      body.innerHTML = content.innerHTML;
      if (typeof dialog.showModal === 'function') {
        dialog.showModal();
      } else {
        dialog.setAttribute('open', 'open');
      }
    }
    function closeResponseModal(id) {
      var dialog = document.getElementById(id);
      if (!dialog) {
        return;
      }
      if (typeof dialog.close === 'function') {
        dialog.close();
      } else {
        dialog.removeAttribute('open');
      }
    }
  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var promptsTemplate = template.Must(template.New("prompts").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp prompts</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    <section class="panel compact">
      <div>{{.TotalPrompts}} prompts{{if .HasMark}} · marked {{.MarkedIndex}}{{end}}</div>
      <div class="instructions">Compile all prompts, from the mark, or an inclusive range.</div>
    </section>
    {{if .Copied}}
    <section class="panel success">compiled to clipboard</section>
    {{end}}
    <section class="panel stack">
      <div class="compile-controls">
        <div>
          <h2 class="section-title">Compile Controls</h2>
          <div class="instructions">Prompt indexes are shown on each card. Range values are inclusive.</div>
        </div>
        <div id="compile-error-anchor"></div>
        <div class="compile-controls-row">
          <label class="label compile-range-field">
            <span>Range start</span>
            <input id="compile-range-start" type="number" min="0" placeholder="0">
          </label>
          <label class="label compile-range-field">
            <span>Range end</span>
            <input id="compile-range-end" type="number" min="0" placeholder="0">
          </label>
          <label class="compile-toggle">
            <input id="compile-update-mark" type="checkbox" checked>
            <span>Update mark after compile</span>
          </label>
        </div>
        <div class="actions">
          <button type="button" class="primary" onclick="startCompileAll()">Compile all</button>
          <button type="button" onclick="startCompileFromMark()">Compile from mark</button>
          <button type="button" onclick="startCompileRange()">Compile range</button>
        </div>
      </div>
    </section>
    <section class="panel prompt-filters">
      <div class="filter-grid">
        <label class="label">
          <span>Search prompts</span>
          <input id="prompt-search" type="search" placeholder="search title or body">
        </label>
        <label class="label">
          <span>From date</span>
          <input id="prompt-date-from" type="date">
        </label>
        <label class="label">
          <span>To date</span>
          <input id="prompt-date-to" type="date">
        </label>
        <div class="actions">
          <button type="button" onclick="clearPromptFilters()">Clear filters</button>
        </div>
      </div>
      <div class="small">Showing <span id="visible-prompt-count">{{.TotalPrompts}}</span> of {{.TotalPrompts}} prompts</div>
    </section>
    <section class="panel stack">
        <div class="prompt-grid" {{if .HasMark}}data-marked-index="{{.MarkedIndex}}"{{end}}>
          {{range .Prompts}}
          <section class="prompt-card {{if .Marked}}marked{{end}}" data-search="{{.SearchText}}" data-date="{{.DateValue}}">
            <div class="prompt-card-copy">
              <strong>{{.Title}}</strong>{{if .Marked}} <span class="mark-badge">marked</span>{{end}}
              <span class="small">#{{.Index}} · {{.Timestamp}}</span>
            </div>
            <div class="actions">
              <button type="button" onclick="openPromptModal('{{.ElementID}}')">View</button>
            </div>
            <div id="{{.ElementID}}-content" hidden>{{.HTMLBody}}</div>
            <dialog id="{{.ElementID}}" class="prompt-modal">
              <div class="modal-shell">
                <div class="modal-header">
                  <div>
                    <div class="summary-title">{{.Title}}</div>
                    <div class="summary-meta">#{{.Index}} · {{.Timestamp}}</div>
                  </div>
                  <button type="button" onclick="closePromptModal('{{.ElementID}}')">Close</button>
                </div>
                <div class="modal-body" id="{{.ElementID}}-body"></div>
                <div class="prompt-actions">
                  <form method="post" action="/prompts/delete">
                    <input type="hidden" name="delete_prompt" value="{{.Index}}">
                    <button type="submit" class="danger">Delete</button>
                  </form>
                </div>
              </div>
            </dialog>
          </section>
          {{else}}
          <section class="panel">
            <div class="muted">No prompts.</div>
          </section>
          {{end}}
        </div>
    </section>
    {{if .Skills}}
    <dialog id="compile-skill-modal" class="prompt-modal">
      <div class="modal-shell">
        <div class="modal-header">
          <div>
            <div class="summary-title">Compile with skills</div>
            <div class="summary-meta">Skills are opt-in. Select only the ones you want in this compilation.</div>
          </div>
          <button type="button" onclick="closeCompileSkillModal()">Close</button>
        </div>
        <div class="modal-body">
          <div class="skill-toggles">
            {{range .Skills}}
            <label class="skill-toggle">
              <input type="checkbox" name="modal_include_skill" value="{{.Name}}" {{if .Included}}checked{{end}}>
              <span>{{.Name}}</span>
            </label>
            {{end}}
          </div>
          <div class="modal-actions">
            <button type="button" class="primary" onclick="confirmCompileWithSkills()">Compile now</button>
          </div>
        </div>
      </div>
    </dialog>
    {{end}}
  </main>
  <script>
    var pendingCompileRequest = null;
    function compileSkillDialog() {
      return document.getElementById('compile-skill-modal');
    }
    function promptFilterElements() {
      return {
        search: document.getElementById('prompt-search'),
        from: document.getElementById('prompt-date-from'),
        to: document.getElementById('prompt-date-to'),
        count: document.getElementById('visible-prompt-count')
      };
    }
    function activePromptSearchTerm() {
      var filters = promptFilterElements();
      return filters.search ? filters.search.value.trim() : '';
    }
    function highlightPromptSearch(container, term) {
      if (!container || !term) {
        return;
      }
      var normalizedTerm = term.toLowerCase();
      var walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT, {
        acceptNode: function(node) {
          if (!node.nodeValue || !node.nodeValue.trim()) {
            return NodeFilter.FILTER_REJECT;
          }
          var parent = node.parentNode;
          if (!parent || /^(SCRIPT|STYLE|MARK|TEXTAREA)$/i.test(parent.nodeName)) {
            return NodeFilter.FILTER_REJECT;
          }
          return node.nodeValue.toLowerCase().indexOf(normalizedTerm) === -1
            ? NodeFilter.FILTER_REJECT
            : NodeFilter.FILTER_ACCEPT;
        }
      });
      var matches = [];
      while (walker.nextNode()) {
        matches.push(walker.currentNode);
      }
      matches.forEach(function(node) {
        var text = node.nodeValue;
        var lower = text.toLowerCase();
        var fragment = document.createDocumentFragment();
        var start = 0;
        while (start < text.length) {
          var index = lower.indexOf(normalizedTerm, start);
          if (index === -1) {
            fragment.appendChild(document.createTextNode(text.slice(start)));
            break;
          }
          if (index > start) {
            fragment.appendChild(document.createTextNode(text.slice(start, index)));
          }
          var mark = document.createElement('mark');
          mark.className = 'search-hit';
          mark.textContent = text.slice(index, index + term.length);
          fragment.appendChild(mark);
          start = index + term.length;
        }
        node.parentNode.replaceChild(fragment, node);
      });
    }
    function applyPromptFilters() {
      var filters = promptFilterElements();
      var searchValue = filters.search ? filters.search.value.trim().toLowerCase() : '';
      var fromValue = filters.from ? filters.from.value : '';
      var toValue = filters.to ? filters.to.value : '';
      var visible = 0;
      document.querySelectorAll('.prompt-card').forEach(function(card) {
        var haystack = (card.getAttribute('data-search') || '').toLowerCase();
        var dateValue = card.getAttribute('data-date') || '';
        var matchesSearch = !searchValue || haystack.indexOf(searchValue) !== -1;
        var matchesFrom = !fromValue || dateValue >= fromValue;
        var matchesTo = !toValue || dateValue <= toValue;
        var show = matchesSearch && matchesFrom && matchesTo;
        card.hidden = !show;
        if (show) {
          visible += 1;
        }
      });
      if (filters.count) {
        filters.count.textContent = String(visible);
      }
    }
    function clearPromptFilters() {
      var filters = promptFilterElements();
      if (filters.search) {
        filters.search.value = '';
      }
      if (filters.from) {
        filters.from.value = '';
      }
      if (filters.to) {
        filters.to.value = '';
      }
      applyPromptFilters();
    }
      function openPromptModal(id) {
        var dialog = document.getElementById(id);
        var body = document.getElementById(id + '-body');
        var content = document.getElementById(id + '-content');
        if (!dialog || !body || !content) {
          return;
        }
        body.innerHTML = content.innerHTML;
        highlightPromptSearch(body, activePromptSearchTerm());
        if (typeof dialog.showModal === 'function') {
          dialog.showModal();
        } else {
          dialog.setAttribute('open', 'open');
        }
      }
      function closePromptModal(id) {
        var dialog = document.getElementById(id);
        if (!dialog) {
          return;
        }
        if (typeof dialog.close === 'function') {
          dialog.close();
        } else {
          dialog.removeAttribute('open');
        }
      }
      function openCompileSkillModal(params) {
        var dialog = compileSkillDialog();
        if (!dialog) {
          submitCompile(params);
          return;
        }
        pendingCompileRequest = {
          params: params
        };
        if (typeof dialog.showModal === 'function') {
          dialog.showModal();
        } else {
          dialog.setAttribute('open', 'open');
        }
      }
      function closeCompileSkillModal() {
        var dialog = compileSkillDialog();
        if (!dialog) {
          pendingCompileRequest = null;
          return;
        }
        if (typeof dialog.close === 'function') {
          dialog.close();
        } else {
          dialog.removeAttribute('open');
        }
        pendingCompileRequest = null;
      }
      function confirmCompileWithSkills() {
        var params = pendingCompileRequest ? new URLSearchParams(pendingCompileRequest.params.toString()) : new URLSearchParams();
        document.querySelectorAll('input[name="modal_include_skill"]').forEach(function(el) {
          if (el.checked) {
            params.append('include_skill', el.value);
          }
        });
        closeCompileSkillModal();
        submitCompile(params);
      }
	    function showCompileError(message) {
	      var anchor = document.getElementById('compile-error-anchor');
	      var error = document.querySelector('.error');
	      if (!error) {
	        error = document.createElement('section');
	        error.className = 'panel error';
	        anchor.parentNode.insertBefore(error, anchor.nextSibling);
	      }
	      error.textContent = message;
	    }
	    function submitCompile(params) {
	      fetch('/compile', {
	        method: 'POST',
	        headers: {
	          'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8'
	        },
	        body: params.toString()
	      }).then(function(response) {
	        return response.json();
	      }).then(function(payload) {
	        if (payload.error) {
	          showCompileError(payload.error);
	          return;
	        }
	        var text = payload.compiled || '';
	        function fallbackCopy() {
	          var area = document.createElement('textarea');
	          area.value = text;
	          document.body.appendChild(area);
	          area.focus();
	          area.select();
	          try {
	            document.execCommand('copy');
	          } catch (err) {
	          }
	          document.body.removeChild(area);
	        }
	        if (navigator.clipboard && navigator.clipboard.writeText) {
	          navigator.clipboard.writeText(text).catch(fallbackCopy).finally(function() {
	            window.location.href = '/prompts?copied=1';
	          });
	        } else {
	          fallbackCopy();
	          window.location.href = '/prompts?copied=1';
	        }
	      });
	    }
      function baseCompileParams(mode) {
        var params = new URLSearchParams();
        params.set('mode', mode);
        var updateMark = document.getElementById('compile-update-mark');
        if (updateMark && updateMark.checked) {
          params.set('update_mark', '1');
        }
        return params;
      }
      function startCompileAll() {
        openCompileSkillModal(baseCompileParams('all'));
      }
      function startCompileFromMark() {
        openCompileSkillModal(baseCompileParams('from_mark'));
      }
      function startCompileRange() {
        var startInput = document.getElementById('compile-range-start');
        var endInput = document.getElementById('compile-range-end');
        var startValue = startInput ? startInput.value.trim() : '';
        var endValue = endInput ? endInput.value.trim() : '';
        if (startValue === '' || endValue === '') {
          showCompileError('range start and end are required');
          return;
        }
        var params = baseCompileParams('range');
        params.set('range_start', startValue);
        params.set('range_end', endValue);
        openCompileSkillModal(params);
      }
    ['prompt-search', 'prompt-date-from', 'prompt-date-to'].forEach(function(id) {
      var el = document.getElementById(id);
      if (!el) {
        return;
      }
      el.addEventListener('input', applyPromptFilters);
      el.addEventListener('change', applyPromptFilters);
    });
    applyPromptFilters();
	  </script>
  ` + liveReloadScript + `
</body>
</html>`))

var projectsTemplate = template.Must(template.New("projects").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>pmp projects</title>
  <style>` + baseStyles + `</style>
  <style>:root { --action: {{.AccentColor}}; }</style>
</head>
<body>
  <main class="shell">
    <nav class="nav">
      ` + navBrand + `
      <div class="nav-links">{{range .Nav}}<a href="{{.Href}}" {{if .Current}}class="current"{{end}}>{{.Label}}</a>{{end}}</div>
    </nav>
    ` + currentProjectBanner + `
    {{if .Switched}}<section class="panel success">project switched</section>{{end}}
    {{if .Created}}<section class="panel success">project created</section>{{end}}
    {{if .Error}}<section class="panel error">{{.Error}}</section>{{end}}
    <section class="panel stack">
      <div>
        <h2 class="section-title">Create New Project</h2>
        <div class="instructions">Choose one of your configured scan roots, create the folder, and initialize PMP in it immediately.</div>
      </div>
      <form method="post" action="/projects/create" class="create-project-grid">
        <label class="label">
          <span>Project name</span>
          <input type="text" name="project_name" value="{{.NewProjectName}}" placeholder="my-new-project">
        </label>
        <label class="label">
          <span>Location</span>
          <select name="project_root">
            {{range .ScanRoots}}
            <option value="{{.}}" {{if eq $.NewProjectRoot .}}selected{{end}}>{{.}}</option>
            {{end}}
          </select>
        </label>
        <div class="actions">
          <button type="submit" class="primary">Create project</button>
        </div>
      </form>
    </section>
    <section class="panel stack">
      <div>
        <h2 class="section-title">Projects On This Machine</h2>
        <div class="instructions">PMP scans only your configured project roots for folders containing <code>.pmp</code> and keeps a local registry of projects you open.</div>
        <div class="instructions">Adjust scan roots in <a href="/settings">settings</a> if you keep projects somewhere else.</div>
      </div>
      <div class="project-list">
        {{range .Projects}}
        <section class="panel project-card {{if .Current}}success{{end}}">
          <div class="project-meta">
            <div class="project-name">{{.Name}}{{if .Current}} <span class="mark-badge">current</span>{{end}}</div>
            <div class="instructions project-path">{{.Path}}</div>
            {{if .LastOpened}}<div class="small">last opened {{.LastOpened}}</div>{{end}}
          </div>
          <div class="actions">
            {{if .Current}}
            <a class="button" href="/new">Open current project</a>
            {{else}}
            <a class="button primary" href="/projects/switch?path={{urlquery .Path}}">Switch to project</a>
            {{end}}
          </div>
        </section>
        {{else}}
        <div class="muted">No PMP projects found yet.</div>
        {{end}}
      </div>
    </section>
  </main>
  ` + liveReloadScript + `
</body>
</html>`))
