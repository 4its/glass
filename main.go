// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
	"go.uber.org/fx"
)

type Config struct {
	BaseURL           string // e.g. https://gitlab.com or https://gitlab.your.company
	Token             string // personal access token with api scope (or read_api + read_repository)
	DestDir           string // where to clone all repos
	Concurrency       int
	DryRun            bool
	Membership        bool // list only projects where user is a member
	MinAccess         int  // 10 Guest, 20 Reporter, 30 Developer, 40 Maintainer, 50 Owner
	IncludeArchived   bool
	VerboseHTTP       bool
	Timeout           time.Duration
	Mirror            bool // true: clone --mirror; false: обычный working tree
	RecurseSubmodules bool // для обычного клона
	CheckoutDefault   bool // после fetch/clone сделать checkout default-ветки проекта
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.BaseURL, "base-url", envOr("GL_BASE_URL", ""), "GitLab base URL (e.g., https://gitlab.com)")
	flag.StringVar(&cfg.Token, "token", envOr("GL_TOKEN", ""), "GitLab Personal Access Token")
	flag.StringVar(&cfg.DestDir, "dest", envOr("DEST_DIR", "./gitlab-backup"), "Destination directory for clones")
	flag.IntVar(&cfg.Concurrency, "j", envOrInt("CONCURRENCY", 4), "Concurrency (workers)")
	flag.BoolVar(&cfg.DryRun, "dry-run", envOrBool("DRY_RUN", false), "Do not clone, just list")
	flag.BoolVar(&cfg.Membership, "membership", envOrBool("MEMBERSHIP", true), "Only projects you are a member of")
	flag.IntVar(&cfg.MinAccess, "min-access", envOrInt("MIN_ACCESS", 10), "Min access level (10..50)")
	flag.BoolVar(&cfg.IncludeArchived, "archived", envOrBool("INCLUDE_ARCHIVED", false), "Include archived projects")
	flag.BoolVar(&cfg.VerboseHTTP, "http-verbose", envOrBool("HTTP_VERBOSE", false), "Log HTTP requests")
	flag.DurationVar(&cfg.Timeout, "timeout", envOrDuration("TIMEOUT", 30*time.Second), "HTTP timeout per request")
	flag.BoolVar(&cfg.Mirror, "mirror", envOrBool("MIRROR", true), "Mirror mode (all refs, bare). If false, do a normal working-tree clone.")
	flag.BoolVar(&cfg.RecurseSubmodules, "recurse-submodules", envOrBool("RECURSE_SUBMODULES", false), "Recurse submodules for non-mirror clones")
	flag.BoolVar(&cfg.CheckoutDefault, "checkout-default", envOrBool("CHECKOUT_DEFAULT", true), "Checkout project default branch after fetch/clone (non-mirror)")
	flag.Parse()
	return cfg
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var x int
		fmt.Sscanf(v, "%d", &x)
		if x != 0 {
			return x
		}
	}
	return def
}
func envOrBool(key string, def bool) bool {
	if v := strings.ToLower(os.Getenv(key)); v != "" {
		return v == "1" || v == "true" || v == "yes"
	}
	return def
}
func envOrDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

// ---------- GitLab API ----------

type GitLabProject struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	Archived          bool   `json:"archived"`
	Visibility        string `json:"visibility"`
	DefaultBranch     string `json:"default_branch"`
}

type GitLabClient interface {
	ListProjects(ctx context.Context) ([]GitLabProject, error)
}

type gitlabClient struct {
	cfg *Config
	cl  *fasthttp.Client
}

func NewGitLabClient(lc fx.Lifecycle, cfg *Config) GitLabClient {
	cl := &fasthttp.Client{
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}
	return &gitlabClient{cfg: cfg, cl: cl}
}

func (g *gitlabClient) ListProjects(ctx context.Context) ([]GitLabProject, error) {
	if g.cfg.BaseURL == "" || g.cfg.Token == "" {
		return nil, errors.New("base-url and token are required")
	}
	base := strings.TrimSuffix(g.cfg.BaseURL, "/")
	// We’ll page through /api/v4/projects with membership & min_access_level
	perPage := 10
	page := 1
	var all []GitLabProject

	for {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		url := fmt.Sprintf("%s/api/v4/projects?simple=true&per_page=%d&page=%d", base, perPage, page)
		if g.cfg.Membership {
			url += "&membership=true"
		}
		if g.cfg.MinAccess > 0 {
			url += fmt.Sprintf("&min_access_level=%d", g.cfg.MinAccess)
		}
		if !g.cfg.IncludeArchived {
			url += "&archived=false"
		}
		req.SetRequestURI(url)
		req.Header.SetMethod("GET")
		req.Header.Set("PRIVATE-TOKEN", g.cfg.Token)

		if g.cfg.VerboseHTTP {
			log.Debug().Str("url", url).Msg("GET")
		}

		err := g.cl.Do(req, resp)
		fasthttp.ReleaseRequest(req)
		if err != nil {
			fasthttp.ReleaseResponse(resp)
			return nil, fmt.Errorf("http error: %w", err)
		}
		if sc := resp.StatusCode(); sc < 200 || sc >= 300 {
			body := string(resp.Body())
			fasthttp.ReleaseResponse(resp)
			return nil, fmt.Errorf("gitlab api status %d: %s", sc, body)
		}
		var projects []GitLabProject
		if err := json.Unmarshal(resp.Body(), &projects); err != nil {
			fasthttp.ReleaseResponse(resp)
			return nil, fmt.Errorf("decode: %w", err)
		}
		all = append(all, projects...)
		log.Debug().Int("fetched", len(projects)).Int("total", len(all)).Int("page", page).Msg("Projects page")
		nextPage := string(resp.Header.Peek("X-Next-Page"))
		fasthttp.ReleaseResponse(resp)
		if nextPage == "" {
			break
		}
		fmt.Sscanf(nextPage, "%d", &page)
		if page == 0 { // safety
			break
		}
	}
	return all, nil
}

// ---------- Cloner ----------

type Cloner interface {
	CloneAll(ctx context.Context, projects []GitLabProject) error
}

type cloner struct {
	cfg *Config
}

func NewCloner(cfg *Config) Cloner {
	return &cloner{cfg: cfg}
}

func (c *cloner) CloneAll(ctx context.Context, projects []GitLabProject) error {
	if len(projects) == 0 {
		log.Warn().Msg("No projects returned by GitLab API (check token/visibility/filters).")
		return nil
	}
	if err := os.MkdirAll(c.cfg.DestDir, 0o755); err != nil {
		return fmt.Errorf("mkdir dest: %w", err)
	}

	type job struct {
		P GitLabProject
	}
	jobs := make(chan job)
	var wg sync.WaitGroup

	var completed int64
	var total = len(projects)
	var mu sync.Mutex
	inProgress := 0

	// Progress ticker
	stopTicker := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				mu.Lock()
				log.Info().
					Int("total", total).
					Int("in_progress", inProgress).
					Int64("done", completed).
					Msg("Progress")
				mu.Unlock()
			case <-stopTicker:
				return
			}
		}
	}()

	// Workers
	workers := c.cfg.Concurrency
	if workers < 1 {
		workers = 1
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := range jobs {
				mu.Lock()
				inProgress++
				mu.Unlock()

				if err := c.processProject(ctx, j.P); err != nil {
					log.Error().Str("project", j.P.PathWithNamespace).Err(err).Msg("Failed")
				} else {
					log.Info().Str("project", j.P.PathWithNamespace).Msg("OK")
				}

				mu.Lock()
				inProgress--
				completed++
				mu.Unlock()
			}
		}(i)
	}

	// Enqueue
	for _, p := range projects {
		if p.Archived && !c.cfg.IncludeArchived {
			continue
		}
		select {
		case <-ctx.Done():
			break
		case jobs <- job{P: p}:
		}
	}
	close(jobs)

	wg.Wait()
	close(stopTicker)

	log.Info().Int("total", total).Int64("done", completed).Msg("Finished")
	return ctx.Err()
}

func (c *cloner) processProject(ctx context.Context, p GitLabProject) error {
	var path string
	if c.cfg.Mirror {
		path = filepath.Join(c.cfg.DestDir, p.PathWithNamespace) + ".git"
	} else {
		path = filepath.Join(c.cfg.DestDir, p.PathWithNamespace) // рабочее дерево
	}
	url := c.buildHTTPURLWithToken(p.HTTPURLToRepo)

	// Ensure parent dir
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir parents: %w", err)
	}

	if c.cfg.DryRun {
		log.Info().
			Str("project", p.PathWithNamespace).
			Str("dest", path).
			Bool("mirror", c.cfg.Mirror).
			Msg("(dry-run) would clone")
		return nil
	}

	if exists(path) {
		if c.cfg.Mirror {
			// обновление bare mirror
			log.Info().Str("project", p.PathWithNamespace).Msg("Updating existing mirror")
			return runGit(ctx, path, "remote", "update", "--prune")
		}
		// обновление рабочего дерева
		log.Info().Str("project", p.PathWithNamespace).Msg("Fetching existing working tree")
		// убедимся, что remote URL актуален (обновим токен при необходимости)
		_ = runGit(ctx, path, "remote", "set-url", "origin", url)
		if err := runGit(ctx, path, "fetch", "--all", "--tags", "--prune"); err != nil {
			return err
		}
		if c.cfg.CheckoutDefault && p.DefaultBranch != "" {
			// создаём/двигаем локальную ветку к origin/default
			_ = runGit(ctx, path, "checkout", "-B", p.DefaultBranch, "origin/"+p.DefaultBranch)
			// подтянуть fast-forward на случай отставания
			_ = runGit(ctx, path, "pull", "--ff-only")
		}
		return nil
	}

	// fresh clone
	if c.cfg.Mirror {
		log.Info().Str("project", p.PathWithNamespace).Str("dest", path).Msg("Cloning mirror")
		return runGit(ctx, "", "clone", "--mirror", url, path)
	}
	// обычный клон с рабочим деревом
	log.Info().Str("project", p.PathWithNamespace).Str("dest", path).Msg("Cloning working tree")
	args := []string{"clone", "--no-single-branch", url, path}
	if c.cfg.RecurseSubmodules {
		args = []string{"clone", "--no-single-branch", "--recurse-submodules", url, path}
	}
	if err := runGit(ctx, "", args...); err != nil {
		return err
	}
	// доп. унификация: получить все remote-tracking ветки/теги и запрунить
	if err := runGit(ctx, path, "fetch", "--all", "--tags", "--prune"); err != nil {
		return err
	}
	// checkout default ветки, если знаем её имя
	if c.cfg.CheckoutDefault && p.DefaultBranch != "" {
		_ = runGit(ctx, path, "checkout", "-B", p.DefaultBranch, "origin/"+p.DefaultBranch)
	}
	return nil
}

func (c *cloner) buildHTTPURLWithToken(httpURL string) string {
	// inject token in https url as oauth2:TOKEN@
	// e.g. https://oauth2:TOKEN@gitlab.com/group/repo.git
	// Avoid printing token in logs EVER.
	if !strings.HasPrefix(httpURL, "http://") && !strings.HasPrefix(httpURL, "https://") {
		return httpURL // fallback (unlikely)
	}
	scheme := "https://"
	if strings.HasPrefix(httpURL, "http://") {
		scheme = "http://"
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(httpURL, "https://"), "http://")
	return scheme + "oauth2:" + c.cfg.Token + "@" + rest
}

func runGit(ctx context.Context, workdir string, args ...string) error {
	var cmd *exec.Cmd
	if workdir == "" {
		cmd = exec.CommandContext(ctx, "git", args...)
	} else {
		cmd = exec.CommandContext(ctx, "git", append([]string{"-C", workdir}, args...)...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	// Compact log (avoid gigantic spam), but keep useful details
	outStr := strings.TrimSpace(limit(stdout.String(), 8<<10)) // 8KB limit
	errStr := strings.TrimSpace(limit(stderr.String(), 8<<10))

	ev := log.Info()
	if err != nil {
		ev = log.Error().Err(err)
	}
	ev = ev.Str("cmd", "git "+strings.Join(args, " ")).
		Str("workdir", workdir).
		Dur("duration", dur)

	if outStr != "" {
		ev = ev.Str("stdout", outStr)
	}
	if errStr != "" {
		ev = ev.Str("stderr", errStr)
	}
	ev.Msg("git")
	return err
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func limit(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// ---------- App ----------

type App struct {
	fx.In
	Lc     fx.Lifecycle
	Cfg    *Config
	API    GitLabClient
	Cloner Cloner
}

func NewConfig() *Config {
	cfg := parseFlags()
	// Logging init
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
	level := zerolog.InfoLevel
	if os.Getenv("DEBUG") == "1" {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)
	return cfg
}

func Run(app App) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Warn().Str("signal", sig.String()).Msg("Shutting down...")
		cancel()
	}()

	start := time.Now()
	log.Info().Msg("Listing GitLab projects...")
	projects, err := app.API.ListProjects(ctx)
	if err != nil {
		return err
	}
	log.Info().Int("count", len(projects)).Msg("Projects fetched")

	if err := app.Cloner.CloneAll(ctx, projects); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info().Dur("total", time.Since(start)).Msg("Done")
	return nil
}

func main() {
	app := fx.New(
		fx.Provide(
			NewConfig,
			NewGitLabClient,
			NewCloner,
		),
		fx.Invoke(Run),
	)

	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("fx start failed")
	}
	// К этому моменту Run уже выполнился (он вызывается на старте).
	if err := app.Stop(ctx); err != nil {
		log.Fatal().Err(err).Msg("fx stop failed")
	}
}
