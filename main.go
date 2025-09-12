// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	Timeout           time.Duration // HTTP timeout per request
	Mirror            bool          // true: clone --mirror; false: обычный working tree
	RecurseSubmodules bool          // для обычного клона
	CheckoutDefault   bool          // после fetch/clone сделать checkout default-ветки проекта
	UseSSH            bool          // использовать SSHURLToRepo вместо HTTP + PAT
	SafeUpdate        bool          // не трогать грязные working tree; только fetch
	ForceReset        bool          // жёстко синхронизировать на origin/<default>
	IncludeNS         string        // regexp include по path_with_namespace
	ExcludeNS         string        // regexp exclude по path_with_namespace
	Since             time.Duration // брать проекты с активностью за период (0 = все)
	MaxSizeMB         int           // пропускать проекты > N МБ (по statistics.storage_size)
	PruneLocal        bool          // удалить локальные репозитории, которых нет в API
	GitTimeout        time.Duration // таймаут на одну git-команду
	UseNetrcAPI       bool          // читать PAT для API из ~/.netrc, если -token пуст
	UseNetrcGit       bool          // не встраивать PAT в HTTPS-URL, полагаться на ~/.netrc для git
	NetrcPath         string        // путь к .netrc (по умолчанию ~/.netrc)
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
	flag.BoolVar(&cfg.UseSSH, "ssh", envOrBool("SSH_MODE", false), "Use SSH clone URLs (requires SSH keys configured)")
	flag.BoolVar(&cfg.SafeUpdate, "safe-update", envOrBool("SAFE_UPDATE", true), "Skip checkout/pull if working tree is dirty")
	flag.BoolVar(&cfg.ForceReset, "force-reset", envOrBool("FORCE_RESET", false), "Hard reset to origin/<default> after fetch (dangerous)")
	flag.StringVar(&cfg.IncludeNS, "include", envOr("INCLUDE", ""), "Regexp filter for path_with_namespace to include")
	flag.StringVar(&cfg.ExcludeNS, "exclude", envOr("EXCLUDE", ""), "Regexp filter for path_with_namespace to exclude")
	flag.DurationVar(&cfg.Since, "since", envOrDuration("SINCE", 0), "Only projects active within this duration (0=all)")
	flag.IntVar(&cfg.MaxSizeMB, "max-size-mb", envOrInt("MAX_SIZE_MB", 0), "Skip projects larger than N MB (0=off)")
	flag.BoolVar(&cfg.PruneLocal, "prune-local", envOrBool("PRUNE_LOCAL", false), "Delete local repos that are no longer accessible/returned by API")
	flag.DurationVar(&cfg.GitTimeout, "git-timeout", envOrDuration("GIT_TIMEOUT", 10*time.Minute), "Timeout for a single git command")
	flag.BoolVar(&cfg.UseNetrcAPI, "use-netrc-api", envOrBool("USE_NETRC_API", true), "Load API token from ~/.netrc if -token is empty")
	flag.BoolVar(&cfg.UseNetrcGit, "use-netrc-git", envOrBool("USE_NETRC_GIT", true), "Let git use ~/.netrc for HTTPS auth (do not embed PAT)")
	flag.StringVar(&cfg.NetrcPath, "netrc", envOr("NETRC", defaultNetrcPath()), "Path to .netrc")
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
	LastActivityAtRaw string `json:"last_activity_at"`
	Statistics        *struct {
		StorageSize int64 `json:"storage_size"`
	} `json:"statistics,omitempty"`
}

func (p GitLabProject) LastActivityAt() time.Time {
	if p.LastActivityAtRaw == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, p.LastActivityAtRaw)
	return t
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

// do performs HTTP with retries/backoff for 429/5xx
func (g *gitlabClient) do(req *fasthttp.Request, resp *fasthttp.Response) error {
	sleep := 100 * time.Millisecond
	for i := 0; i < 8; i++ {
		if err := g.cl.Do(req, resp); err != nil {
			time.Sleep(sleep)
			sleep *= 2
			continue
		}
		sc := resp.StatusCode()
		if sc == 429 || sc >= 500 {
			if ra := resp.Header.Peek("Retry-After"); len(ra) > 0 {
				if n, err := strconv.Atoi(string(ra)); err == nil && n > 0 {
					time.Sleep(time.Duration(n) * time.Second)
				}
			} else {
				time.Sleep(sleep)
				sleep *= 2
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("http retries exhausted")
}

func (g *gitlabClient) ListProjects(ctx context.Context) ([]GitLabProject, error) {
	if g.cfg.BaseURL == "" || g.cfg.Token == "" {
		return nil, errors.New("base-url and token are required")
	}
	base := strings.TrimSuffix(g.cfg.BaseURL, "/")
	perPage := 10
	page := 1
	var all []GitLabProject

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		url := fmt.Sprintf("%s/api/v4/projects?simple=true&statistics=true&order_by=last_activity_at&sort=desc&per_page=%d&page=%d", base, perPage, page)
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

		err := g.do(req, resp)
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
		if page == 0 {
			break
		}
	}

	// стабильный порядок: сначала самые активные (дополнительно к order_by)
	sort.Slice(all, func(i, j int) bool { return all[i].LastActivityAt().After(all[j].LastActivityAt()) })
	return all, nil
}

// ---------- Cloner ----------

type Cloner interface {
	CloneAll(ctx context.Context, projects []GitLabProject) error
}

type cloner struct{ cfg *Config }

func NewCloner(cfg *Config) Cloner { return &cloner{cfg: cfg} }

func (c *cloner) CloneAll(ctx context.Context, projects []GitLabProject) error {
	if len(projects) == 0 {
		log.Warn().Msg("No projects returned by GitLab API (check token/visibility/filters).")
		return nil
	}
	if err := os.MkdirAll(c.cfg.DestDir, 0o755); err != nil {
		return fmt.Errorf("mkdir dest: %w", err)
	}

	// compile filters
	var incRe, excRe *regexp.Regexp
	if c.cfg.IncludeNS != "" {
		incRe = regexp.MustCompile(c.cfg.IncludeNS)
	}
	if c.cfg.ExcludeNS != "" {
		excRe = regexp.MustCompile(c.cfg.ExcludeNS)
	}

	type job struct{ P GitLabProject }
	jobs := make(chan job)
	var wg sync.WaitGroup

	var completed int64
	total := 0
	for _, p := range projects {
		if p.Archived && !c.cfg.IncludeArchived {
			continue
		}
		if incRe != nil && !incRe.MatchString(p.PathWithNamespace) {
			continue
		}
		if excRe != nil && excRe.MatchString(p.PathWithNamespace) {
			continue
		}
		if c.cfg.MaxSizeMB > 0 && p.Statistics != nil && p.Statistics.StorageSize > int64(c.cfg.MaxSizeMB)*1024*1024 {
			continue
		}
		if c.cfg.Since > 0 && !p.LastActivityAt().IsZero() && time.Since(p.LastActivityAt()) > c.cfg.Since {
			continue
		}
		total++
	}

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
				log.Info().Int("total", total).Int("in_progress", inProgress).Int64("done", completed).Msg("Progress")
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

	// Enqueue after filtering
	for _, p := range projects {
		if p.Archived && !c.cfg.IncludeArchived {
			continue
		}
		if incRe != nil && !incRe.MatchString(p.PathWithNamespace) {
			continue
		}
		if excRe != nil && excRe.MatchString(p.PathWithNamespace) {
			continue
		}
		if c.cfg.MaxSizeMB > 0 && p.Statistics != nil && p.Statistics.StorageSize > int64(c.cfg.MaxSizeMB)*1024*1024 {
			continue
		}
		if c.cfg.Since > 0 && !p.LastActivityAt().IsZero() && time.Since(p.LastActivityAt()) > c.cfg.Since {
			continue
		}
		select {
		case <-ctx.Done():
			continue
		case jobs <- job{P: p}:
		}
	}
	close(jobs)

	wg.Wait()
	close(stopTicker)

	// prune local if requested
	if c.cfg.PruneLocal {
		live := make(map[string]struct{}, total)
		for _, p := range projects {
			if p.Archived && !c.cfg.IncludeArchived {
				continue
			}
			if incRe != nil && !incRe.MatchString(p.PathWithNamespace) {
				continue
			}
			if excRe != nil && excRe.MatchString(p.PathWithNamespace) {
				continue
			}
			if c.cfg.MaxSizeMB > 0 && p.Statistics != nil && p.Statistics.StorageSize > int64(c.cfg.MaxSizeMB)*1024*1024 {
				continue
			}
			if c.cfg.Since > 0 && !p.LastActivityAt().IsZero() && time.Since(p.LastActivityAt()) > c.cfg.Since {
				continue
			}
			pth := filepath.Join(c.cfg.DestDir, p.PathWithNamespace)
			if c.cfg.Mirror {
				pth += ".git"
			}
			live[pth] = struct{}{}
		}
		filepath.WalkDir(c.cfg.DestDir, func(pth string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			var candidate string
			if c.cfg.Mirror && strings.HasSuffix(pth, ".git") {
				candidate = pth
			}
			if !c.cfg.Mirror && fileExists(filepath.Join(pth, ".git")) {
				candidate = pth
			}
			if candidate == "" {
				return nil
			}
			if _, ok := live[candidate]; !ok {
				log.Warn().Str("local", candidate).Msg("Prune local repo (not in API)")
				_ = os.RemoveAll(candidate)
			}
			return nil
		})
	}

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

	var url string
	if c.cfg.UseSSH {
		url = p.SSHURLToRepo
	} else {
		if c.cfg.UseNetrcGit {
			// оставляем \"чистый\" HTTPS — git возьмёт креды из ~/.netrc
			url = p.HTTPURLToRepo
		} else {
			url = c.buildHTTPURLWithToken(p.HTTPURLToRepo)
		}
	}

	// Ensure parent dir
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir parents: %w", err)
	}

	if c.cfg.DryRun {
		log.Info().Str("project", p.PathWithNamespace).Str("dest", path).Bool("mirror", c.cfg.Mirror).Msg("(dry-run) would clone")
		return nil
	}

	if exists(path) {
		if c.cfg.Mirror {
			log.Info().Str("project", p.PathWithNamespace).Msg("Updating existing mirror")
			return runGit(ctx, c.cfg.GitTimeout, path, "remote", "update", "--prune")
		}
		// обновление рабочего дерева
		log.Info().Str("project", p.PathWithNamespace).Msg("Fetching existing working tree")
		_ = runGit(ctx, c.cfg.GitTimeout, path, "remote", "set-url", "origin", url)
		if err := runGit(ctx, c.cfg.GitTimeout, path, "fetch", "--all", "--tags", "--prune"); err != nil {
			return err
		}
		if c.cfg.CheckoutDefault && p.DefaultBranch != "" {
			if c.cfg.SafeUpdate {
				// грязное дерево? пропускаем checkout/pull
				if err := runGit(ctx, c.cfg.GitTimeout, path, "diff", "--quiet"); err != nil || fileExists(filepath.Join(path, ".git", "MERGE_HEAD")) {
					log.Warn().Str("project", p.PathWithNamespace).Msg("Dirty or in-merge state, skip checkout/pull (safe-update)")
					return nil
				}
			}
			if c.cfg.ForceReset {
				_ = runGit(ctx, c.cfg.GitTimeout, path, "checkout", "-B", p.DefaultBranch, "origin/"+p.DefaultBranch)
				_ = runGit(ctx, c.cfg.GitTimeout, path, "reset", "--hard", "origin/"+p.DefaultBranch)
			} else {
				_ = runGit(ctx, c.cfg.GitTimeout, path, "checkout", "-B", p.DefaultBranch, "origin/"+p.DefaultBranch)
				_ = runGit(ctx, c.cfg.GitTimeout, path, "pull", "--ff-only")
			}
		}
		return nil
	}

	// fresh clone
	if c.cfg.Mirror {
		log.Info().Str("project", p.PathWithNamespace).Str("dest", path).Msg("Cloning mirror")
		return runGit(ctx, c.cfg.GitTimeout, "", "clone", "--mirror", url, path)
	}
	log.Info().Str("project", p.PathWithNamespace).Str("dest", path).Msg("Cloning working tree")
	args := []string{"clone", "--no-single-branch", url, path}
	if c.cfg.RecurseSubmodules {
		args = []string{"clone", "--no-single-branch", "--recurse-submodules", url, path}
	}
	if err := runGit(ctx, c.cfg.GitTimeout, "", args...); err != nil {
		return err
	}
	if err := runGit(ctx, c.cfg.GitTimeout, path, "fetch", "--all", "--tags", "--prune"); err != nil {
		return err
	}
	if c.cfg.CheckoutDefault && p.DefaultBranch != "" {
		_ = runGit(ctx, c.cfg.GitTimeout, path, "checkout", "-B", p.DefaultBranch, "origin/"+p.DefaultBranch)
	}
	return nil
}

func (c *cloner) buildHTTPURLWithToken(httpURL string) string {
	// inject token in https url as oauth2:TOKEN@
	// e.g. https://oauth2:TOKEN@gitlab.com/group/repo.git
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

func runGit(parent context.Context, timeout time.Duration, workdir string, args ...string) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	if workdir != "" {
		args = append([]string{"-C", workdir}, args...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	outStr := strings.TrimSpace(limit(stdout.String(), 8<<10)) // 8KB
	errStr := strings.TrimSpace(limit(stderr.String(), 8<<10))

	ev := log.Info()
	if err != nil {
		ev = log.Error().Err(err)
	}
	ev = ev.Str("cmd", "git "+strings.Join(args, " ")).Str("workdir", workdir).Dur("duration", dur)
	if outStr != "" {
		ev = ev.Str("stdout", outStr)
	}
	if errStr != "" {
		ev = ev.Str("stderr", errStr)
	}
	ev.Msg("git")
	return err
}

func exists(path string) bool  { _, err := os.Stat(path); return err == nil }
func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

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
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
	level := zerolog.InfoLevel
	if os.Getenv("DEBUG") == "1" {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)
	// Если токен не задан и разрешено читать из netrc — попробуем
	if cfg.Token == "" && cfg.UseNetrcAPI && cfg.BaseURL != "" {
		if u, err := url.Parse(cfg.BaseURL); err == nil && u.Host != "" {
			if tok, ok, err := loadTokenFromNetrc(cfg.NetrcPath, u.Host); err == nil && ok && tok != "" {
				cfg.Token = tok
				log.Debug().Str("host", u.Host).Msg("Loaded API token from .netrc")
			}
		}
	}
	log.Debug().Interface("config", cfg).Msg("Configuration")
	return cfg
}

func Run(app App) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { sig := <-sigCh; log.Warn().Str("signal", sig.String()).Msg("Shutting down..."); cancel() }()

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
		fx.Provide(NewConfig, NewGitLabClient, NewCloner),
		fx.Invoke(Run),
	)
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("fx start failed")
	}
	if err := app.Stop(ctx); err != nil {
		log.Fatal().Err(err).Msg("fx stop failed")
	}
}
