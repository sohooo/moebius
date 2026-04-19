package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/cli"
	"github.com/sohooo/moebius/internal/comment"
	"github.com/sohooo/moebius/internal/config"
	"github.com/sohooo/moebius/internal/gitrepo"
)

type repoContext struct {
	Root              string
	Config            config.RepoConfig
	ConfigMeta        config.LoadMetadata
	EffectiveLayout   config.LayoutConfig
	BaseRefName       string
	CurrentClusters   []string
	BaselineClusters  []string
	ChangedClusters   []string
	AvailableClusters []string
}

type doctorCheck struct {
	Level   string
	Message string
}

var (
	openRepo          = gitrepo.Open
	loadRepoConfig    = config.LoadRepoConfigWithMetadata
	newCommentChecker = func() commentService { return comment.New() }
)

func inspectRepo(opts cli.Options) (repoContext, error) {
	repo, err := openRepo(".")
	if err != nil {
		return repoContext{}, err
	}
	cfg, meta, err := loadRepoConfig(repo.Root())
	if err != nil {
		return repoContext{}, err
	}
	layout := cfg.Layout
	layout.ClustersDir = cfg.EffectiveClustersDir(opts.ClustersDir)

	head, err := repo.ResolveCommit("HEAD")
	if err != nil {
		return repoContext{}, err
	}
	baseRefName, baseRef, err := repo.ResolveBaseRef(opts.BaseRef)
	if err != nil {
		return repoContext{}, err
	}
	currentClusters, err := repo.AllClusters(layout.ClustersDir, layout.Apps.File)
	if err != nil {
		return repoContext{}, err
	}
	baselineClusters, err := repo.AllClustersAtCommit(baseRef, layout.ClustersDir, layout.Apps.File)
	if err != nil {
		return repoContext{}, err
	}
	changedClusters, err := repo.ChangedClusters(layout.ClustersDir, baseRef, head)
	if err != nil {
		return repoContext{}, err
	}
	available := unionStrings(currentClusters, baselineClusters)
	if opts.Cluster != "" && !containsString(available, opts.Cluster) {
		if len(available) == 0 {
			return repoContext{}, fmt.Errorf("cluster %q does not exist in the effective layout under %q", opts.Cluster, layout.ClustersDir)
		}
		return repoContext{}, fmt.Errorf("cluster %q does not exist in the effective layout under %q; available clusters: %s", opts.Cluster, layout.ClustersDir, strings.Join(available, ", "))
	}

	return repoContext{
		Root:              repo.Root(),
		Config:            cfg,
		ConfigMeta:        meta,
		EffectiveLayout:   layout,
		BaseRefName:       baseRefName,
		CurrentClusters:   currentClusters,
		BaselineClusters:  baselineClusters,
		ChangedClusters:   changedClusters,
		AvailableClusters: available,
	}, nil
}

func runClusters(stdout io.Writer, opts cli.Options) error {
	ctx, err := inspectRepo(opts)
	if err != nil {
		return err
	}
	clusters := ctx.AvailableClusters
	if opts.Cluster != "" {
		clusters = []string{opts.Cluster}
	}
	if len(clusters) == 0 {
		fmt.Fprintf(stdout, "No clusters discovered under %s (apps file: %s)\n", ctx.EffectiveLayout.ClustersDir, ctx.EffectiveLayout.Apps.File)
		return nil
	}
	fmt.Fprintf(stdout, "Clusters under %s (apps file: %s, base ref: %s)\n", ctx.EffectiveLayout.ClustersDir, ctx.EffectiveLayout.Apps.File, ctx.BaseRefName)
	for _, cluster := range clusters {
		fmt.Fprintf(stdout, "- %s: current=%s baseline=%s changed=%s\n",
			cluster,
			yesNo(containsString(ctx.CurrentClusters, cluster)),
			yesNo(containsString(ctx.BaselineClusters, cluster)),
			yesNo(containsString(ctx.ChangedClusters, cluster)),
		)
	}
	return nil
}

func runDoctor(stdout io.Writer, opts cli.Options) error {
	checks := make([]doctorCheck, 0, 8)
	ctx, err := inspectRepo(opts)
	if err != nil {
		checks = append(checks, doctorCheck{Level: "error", Message: err.Error()})
		printDoctorChecks(stdout, checks)
		return err
	}

	checks = append(checks,
		doctorCheck{Level: "ok", Message: fmt.Sprintf("git repository found: %s", ctx.Root)},
		doctorCheck{Level: "ok", Message: fmt.Sprintf("config loaded from %s", ctx.ConfigMeta.SourceSummary())},
		doctorCheck{Level: "ok", Message: fmt.Sprintf("effective layout: clusters_dir=%s apps_file=%s override_path=%s fallback_path=%s", ctx.EffectiveLayout.ClustersDir, ctx.EffectiveLayout.Apps.File, ctx.EffectiveLayout.Overrides.Path, ctx.EffectiveLayout.Overrides.FallbackPath)},
		doctorCheck{Level: "ok", Message: fmt.Sprintf("field mapping: name=%s namespace=%s project=%s repoURL=%s chart=%s targetRevision=%s", ctx.EffectiveLayout.Apps.Fields.Name, ctx.EffectiveLayout.Apps.Fields.Namespace, ctx.EffectiveLayout.Apps.Fields.Project, ctx.EffectiveLayout.Apps.Fields.RepoURL, ctx.EffectiveLayout.Apps.Fields.Chart, ctx.EffectiveLayout.Apps.Fields.TargetRevision)},
		doctorCheck{Level: "ok", Message: fmt.Sprintf("base ref resolved to %s", ctx.BaseRefName)},
	)
	if _, err := os.Stat(filepath.Join(ctx.Root, ctx.EffectiveLayout.ClustersDir)); err != nil {
		checks = append(checks, doctorCheck{Level: "error", Message: fmt.Sprintf("clusters directory %q is not accessible", ctx.EffectiveLayout.ClustersDir)})
	} else {
		checks = append(checks, doctorCheck{Level: "ok", Message: fmt.Sprintf("clusters directory exists: %s", ctx.EffectiveLayout.ClustersDir)})
	}
	if opts.Cluster != "" {
		checks = append(checks, doctorCheck{Level: "ok", Message: fmt.Sprintf("selected cluster exists: %s", opts.Cluster)})
	} else {
		checks = append(checks, doctorCheck{Level: "ok", Message: fmt.Sprintf("discovered %d cluster(s)", len(ctx.AvailableClusters))})
	}

	if shouldRunGitLabDoctor(opts) {
		status, preflightErr := newCommentChecker().Preflight(context.Background(), cli.Options{
			ProjectID:       opts.ProjectID,
			MergeRequestIID: opts.MergeRequestIID,
			GitLabBaseURL:   opts.GitLabBaseURL,
			GitLabToken:     opts.GitLabToken,
		})
		if preflightErr != nil {
			for _, message := range status.Messages {
				checks = append(checks, doctorCheck{Level: "error", Message: message})
			}
		} else {
			checks = append(checks, doctorCheck{Level: "ok", Message: fmt.Sprintf("GitLab comment preflight passed (token=%s from %s)", status.TokenKind, status.TokenSource)})
		}
	} else {
		checks = append(checks, doctorCheck{Level: "warn", Message: "GitLab comment checks skipped; no merge request context or token detected"})
	}

	printDoctorChecks(stdout, checks)
	for _, check := range checks {
		if check.Level == "error" {
			return fmt.Errorf("doctor found blocking issues")
		}
	}
	return nil
}

func printDoctorChecks(stdout io.Writer, checks []doctorCheck) {
	for _, check := range checks {
		fmt.Fprintf(stdout, "[%s] %s\n", check.Level, check.Message)
	}
}

func shouldRunGitLabDoctor(opts cli.Options) bool {
	values := []string{
		opts.ProjectID,
		opts.MergeRequestIID,
		opts.GitLabBaseURL,
		opts.GitLabToken,
		os.Getenv("CI_PROJECT_ID"),
		os.Getenv("CI_MERGE_REQUEST_IID"),
		os.Getenv("CI_API_V4_URL"),
		os.Getenv("CI_SERVER_URL"),
		os.Getenv("GITLAB_TOKEN"),
		os.Getenv("GITLAB_PRIVATE_TOKEN"),
		os.Getenv("GITLAB_API_TOKEN"),
		os.Getenv("CI_JOB_TOKEN"),
	}
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func formatNoChangesMessage(ctx repoContext) string {
	return fmt.Sprintf("No affected clusters.\n- Effective clusters dir: %s\n- Base ref: %s\n- Hint: use --all-clusters or run `mobius clusters` to inspect available clusters.\n", ctx.EffectiveLayout.ClustersDir, ctx.BaseRefName)
}

func unionStrings(left, right []string) []string {
	set := map[string]struct{}{}
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
