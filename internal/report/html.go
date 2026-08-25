package report

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sohooo/moebius/internal/diff"
	"github.com/sohooo/moebius/internal/output"
	"github.com/sohooo/moebius/internal/severity"
	"github.com/sohooo/moebius/internal/validate"
)

const artifactHTMLFilename = "report.html"

type artifactHTMLView struct {
	Stats       artifactHTMLStats
	Clusters    []artifactHTMLCluster
	Diagnostics []artifactHTMLDiagnostic
	Run         artifactHTMLRun
}

type artifactHTMLStats struct {
	Clusters    int
	Charts      int
	Resources   int
	Added       int
	Removed     int
	Changed     int
	Critical    int
	High        int
	Warnings    int
	Invalid     int
	Unvalidated int
}

type artifactHTMLCluster struct {
	ID      string
	Name    string
	Charts  []artifactHTMLChart
	Added   int
	Removed int
	Changed int
}

type artifactHTMLChart struct {
	ID              string
	Name            string
	Namespace       string
	Resources       []artifactHTMLResource
	Warnings        []string
	RenderWarning   string
	BaselineVersion string
	CurrentVersion  string
	Added           int
	Removed         int
	Changed         int
	Severity        string
	Open            bool
}

type artifactHTMLResource struct {
	ID                 string
	Kind               string
	Name               string
	Namespace          string
	State              string
	Severity           string
	SearchText         string
	Summary            string
	Findings           []artifactHTMLFinding
	ValidationStatus   string
	ValidationCoverage string
	ValidationSource   string
	ValidationFindings []artifactHTMLValidationFinding
	ChangePaths        []artifactHTMLChangePath
	Semantic           []artifactHTMLDiffLine
	Raw                []artifactHTMLDiffLine
	Open               bool
}

type artifactHTMLFinding struct {
	Level    string
	Category string
	Reason   string
	Path     string
}

type artifactHTMLValidationFinding struct {
	Status  string
	Source  string
	Message string
	Path    string
}

type artifactHTMLChangePath struct {
	State string
	Path  string
}

type artifactHTMLDiffLine struct {
	Class string
	Text  string
}

type artifactHTMLDiagnostic struct {
	Level   string
	Path    string
	Content string
}

type artifactHTMLRun struct {
	Available         bool
	Mode              string
	BaseRef           string
	HeadSHA           string
	MergeBaseSHA      string
	ConfigSources     string
	ChangedPathsCount int
	SelectedClusters  string
	CommentMode       string
	Validate          bool
}

func writeArtifactHTML(outputDir string, reports []output.ClusterReport, summary *runSummary) error {
	if outputDir == "" {
		return nil
	}
	view := buildArtifactHTMLView(outputDir, reports, summary)
	path := filepath.Join(outputDir, artifactHTMLFilename)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := artifactHTMLTemplate.Execute(file, view); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func buildArtifactHTMLView(outputDir string, reports []output.ClusterReport, summary *runSummary) artifactHTMLView {
	view := artifactHTMLView{Run: artifactHTMLRunView(summary)}
	for clusterIndex, report := range reports {
		cluster := artifactHTMLCluster{
			ID:      fmt.Sprintf("cluster-%d", clusterIndex+1),
			Name:    report.Name,
			Added:   report.Added,
			Removed: report.Removed,
			Changed: report.Changed,
		}
		view.Stats.Clusters++
		view.Stats.Added += report.Added
		view.Stats.Removed += report.Removed
		view.Stats.Changed += report.Changed
		for chartIndex, chartReport := range report.Charts {
			chart := artifactHTMLChart{
				ID:              fmt.Sprintf("cluster-%d-chart-%d", clusterIndex+1, chartIndex+1),
				Name:            chartReport.Name,
				Namespace:       emptyArtifactValue(chartReport.Namespace),
				Warnings:        append([]string(nil), chartReport.Warnings...),
				RenderWarning:   chartReport.RenderWarning,
				BaselineVersion: chartReport.BaselineTargetRevision,
				CurrentVersion:  chartReport.CurrentTargetRevision,
				Severity:        string(severity.LevelInfo),
			}
			view.Stats.Charts++
			view.Stats.Warnings += len(chartReport.Warnings)
			if chartReport.RenderWarning != "" {
				view.Stats.Warnings++
				chart.Open = true
			}
			for resourceIndex, resourceReport := range chartReport.Resources {
				resource := artifactHTMLResourceView(clusterIndex, chartIndex, resourceIndex, report.Name, chartReport, resourceReport)
				chart.Resources = append(chart.Resources, resource)
				view.Stats.Resources++
				switch resourceReport.State {
				case "added":
					chart.Added++
				case "removed":
					chart.Removed++
				default:
					chart.Changed++
				}
				if severity.Rank(resourceReport.Assessment.Level) > severity.Rank(severity.Level(chart.Severity)) {
					chart.Severity = string(resourceReport.Assessment.Level)
				}
				switch resourceReport.Assessment.Level {
				case severity.LevelCritical:
					view.Stats.Critical++
				case severity.LevelHigh:
					view.Stats.High++
				}
				if resourceReport.Validation.Status == validate.StatusError {
					view.Stats.Invalid++
				}
				if resourceReport.Validation.Coverage == validate.CoverageUnvalidated {
					view.Stats.Unvalidated++
				}
			}
			if severity.Rank(severity.Level(chart.Severity)) >= severity.Rank(severity.LevelHigh) {
				chart.Open = true
			}
			cluster.Charts = append(cluster.Charts, chart)
		}
		view.Clusters = append(view.Clusters, cluster)
	}
	view.Diagnostics = artifactHTMLDiagnostics(outputDir)
	return view
}

func artifactHTMLResourceView(clusterIndex, chartIndex, resourceIndex int, clusterName string, chart output.ChartReport, resource output.ResourceReport) artifactHTMLResource {
	semantic, err := diff.RenderSemanticMarkdown(resource.Result.Changes)
	if err != nil || strings.TrimSpace(semantic) == "" {
		semantic = resource.Semantic
	}
	view := artifactHTMLResource{
		ID:                 fmt.Sprintf("cluster-%d-chart-%d-resource-%d", clusterIndex+1, chartIndex+1, resourceIndex+1),
		Kind:               resource.Kind,
		Name:               resource.Name,
		Namespace:          emptyArtifactValue(resource.Namespace),
		State:              resource.State,
		Severity:           string(resource.Assessment.Level),
		ValidationStatus:   emptyArtifactValue(string(resource.Validation.Status)),
		ValidationCoverage: emptyArtifactValue(string(resource.Validation.Coverage)),
		ValidationSource:   emptyArtifactValue(string(resource.Validation.SchemaSource)),
		Semantic:           artifactHTMLDiffLines(semantic),
		Raw:                artifactHTMLDiffLines(resource.Result.RawDiff),
		Open:               severity.Rank(resource.Assessment.Level) >= severity.Rank(severity.LevelHigh) || resource.Validation.Status == validate.StatusError,
	}
	var searchParts = []string{clusterName, chart.Name, chart.Namespace, resource.Kind, resource.Name, resource.Namespace, resource.State, string(resource.Assessment.Level), string(resource.Validation.Status), string(resource.Validation.Coverage)}
	for _, finding := range resource.Assessment.Findings {
		view.Findings = append(view.Findings, artifactHTMLFinding{
			Level:    string(finding.Level),
			Category: finding.Category,
			Reason:   finding.Reason,
			Path:     finding.Path,
		})
		searchParts = append(searchParts, finding.Category, finding.Reason, finding.Path)
	}
	for _, finding := range resource.Validation.Findings {
		view.ValidationFindings = append(view.ValidationFindings, artifactHTMLValidationFinding{
			Status:  string(finding.Status),
			Source:  string(finding.Source),
			Message: finding.Message,
			Path:    finding.Path,
		})
		searchParts = append(searchParts, string(finding.Source), finding.Message, finding.Path)
	}
	for _, change := range resource.Result.Changes {
		path := diff.PathString(change.Path)
		view.ChangePaths = append(view.ChangePaths, artifactHTMLChangePath{State: change.State, Path: path})
		searchParts = append(searchParts, path)
	}
	if len(resource.Assessment.Findings) > 0 {
		view.Summary = resource.Assessment.Findings[0].Reason
	} else {
		view.Summary = resource.State + " resource"
	}
	view.SearchText = strings.ToLower(strings.Join(searchParts, " "))
	return view
}

func artifactHTMLDiffLines(text string) []artifactHTMLDiffLine {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	out := make([]artifactHTMLDiffLine, 0, len(lines))
	for _, line := range lines {
		class := "context"
		switch {
		case strings.HasPrefix(line, "@@"):
			class = "hunk"
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "# Path:"):
			class = "meta"
		case strings.HasPrefix(line, "+"):
			class = "added"
		case strings.HasPrefix(line, "-"):
			class = "removed"
		}
		out = append(out, artifactHTMLDiffLine{Class: class, Text: line})
	}
	return out
}

func artifactHTMLDiagnostics(outputDir string) []artifactHTMLDiagnostic {
	var out []artifactHTMLDiagnostic
	for _, level := range []string{"errors", "warnings"} {
		for _, name := range listArtifactFiles(filepath.Join(outputDir, level)) {
			path := filepath.Join(outputDir, level, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			out = append(out, artifactHTMLDiagnostic{
				Level:   strings.TrimSuffix(level, "s"),
				Path:    filepath.ToSlash(filepath.Join(level, name)),
				Content: strings.TrimSpace(string(data)),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func artifactHTMLRunView(summary *runSummary) artifactHTMLRun {
	if summary == nil {
		return artifactHTMLRun{}
	}
	return artifactHTMLRun{
		Available:         true,
		Mode:              summary.Mode,
		BaseRef:           summary.BaseRef,
		HeadSHA:           summary.HeadSHA,
		MergeBaseSHA:      summary.MergeBaseSHA,
		ConfigSources:     summary.ConfigSources,
		ChangedPathsCount: summary.ChangedPathsCount,
		SelectedClusters:  strings.Join(summary.SelectedClusters, ", "),
		CommentMode:       summary.Options.CommentMode,
		Validate:          summary.Options.Validate,
	}
}

func emptyArtifactValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

var artifactHTMLTemplate = template.Must(template.New("artifact-report").Parse(artifactHTMLTemplateText))

const artifactHTMLTemplateText = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>møbius Diff Report</title>
<style>
:root { color-scheme: light dark; --bg:#f6f8fa; --panel:#fff; --panel-2:#f0f3f6; --text:#1f2328; --muted:#59636e; --border:#d0d7de; --accent:#8250df; --accent-soft:#f3edff; --critical:#cf222e; --high:#bc4c00; --medium:#9a6700; --low:#0969da; --info:#57606a; --added:#1a7f37; --removed:#cf222e; --shadow:0 1px 2px rgba(31,35,40,.08),0 8px 24px rgba(140,149,159,.12); }
@media (prefers-color-scheme:dark) { :root { --bg:#0d1117; --panel:#161b22; --panel-2:#21262d; --text:#e6edf3; --muted:#8b949e; --border:#30363d; --accent:#a371f7; --accent-soft:#2b2140; --critical:#ff7b72; --high:#ffa657; --medium:#d29922; --low:#58a6ff; --info:#8b949e; --added:#3fb950; --removed:#ff7b72; --shadow:none; } }
* { box-sizing:border-box; }
html { scroll-behavior:smooth; }
body { margin:0; background:var(--bg); color:var(--text); font:14px/1.5 ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }
a { color:var(--accent); text-decoration:none; } a:hover { text-decoration:underline; }
code,pre,.mono { font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
.layout { display:grid; grid-template-columns:260px minmax(0,1fr); min-height:100vh; }
.sidebar { position:sticky; top:0; height:100vh; overflow:auto; padding:24px 18px; border-right:1px solid var(--border); background:var(--panel); }
.brand { font-size:20px; font-weight:750; letter-spacing:-.02em; margin-bottom:4px; }
.eyebrow { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.08em; }
.nav { margin:24px 0; } .nav-cluster { margin:16px 0; } .nav-cluster>a { color:var(--text); font-weight:650; }
.nav-chart { display:block; color:var(--muted); padding:4px 0 4px 12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.sidebar-links { border-top:1px solid var(--border); padding-top:14px; display:grid; gap:7px; }
.content { min-width:0; padding:34px clamp(20px,4vw,64px) 80px; }
.hero { max-width:1400px; margin:0 auto 24px; }
h1 { margin:4px 0 8px; font-size:34px; letter-spacing:-.035em; line-height:1.15; }
h2 { font-size:24px; letter-spacing:-.02em; } h3 { font-size:17px; }
.lede { color:var(--muted); max-width:760px; margin:0; }
.stats { display:grid; grid-template-columns:repeat(auto-fit,minmax(125px,1fr)); gap:10px; margin:24px 0; }
.stat { background:var(--panel); border:1px solid var(--border); border-radius:10px; padding:13px 15px; box-shadow:var(--shadow); }
.stat strong { display:block; font-size:22px; line-height:1.2; } .stat span { color:var(--muted); font-size:12px; }
.stat.critical strong { color:var(--critical); } .stat.high strong { color:var(--high); }
.toolbar { position:sticky; top:12px; z-index:10; display:flex; flex-wrap:wrap; gap:8px; align-items:center; background:color-mix(in srgb,var(--panel) 94%,transparent); backdrop-filter:blur(10px); border:1px solid var(--border); border-radius:12px; padding:10px; box-shadow:var(--shadow); }
input,select,button { color:var(--text); background:var(--panel); border:1px solid var(--border); border-radius:7px; padding:8px 10px; font:inherit; }
input { flex:1 1 300px; min-width:180px; } button { cursor:pointer; } button:hover { border-color:var(--accent); }
.match-count { color:var(--muted); margin-left:auto; white-space:nowrap; }
.report { max-width:1400px; margin:0 auto; }
.cluster { scroll-margin-top:84px; margin:34px 0; }
.cluster-head { display:flex; align-items:baseline; gap:12px; border-bottom:1px solid var(--border); margin-bottom:14px; }
.cluster-head h2 { margin:0 0 9px; } .cluster-head .counts { color:var(--muted); }
.chart { scroll-margin-top:84px; background:var(--panel); border:1px solid var(--border); border-radius:12px; margin:12px 0; box-shadow:var(--shadow); overflow:hidden; }
.chart>summary,.resource>summary { cursor:pointer; list-style:none; } .chart>summary::-webkit-details-marker,.resource>summary::-webkit-details-marker { display:none; }
.chart>summary { padding:15px 17px; display:flex; gap:12px; align-items:center; }
.chart>summary:before,.resource>summary:before { content:""; width:7px; height:7px; border-right:2px solid var(--muted); border-bottom:2px solid var(--muted); transform:rotate(-45deg); transition:transform .12s ease; flex:none; }
.chart[open]>summary:before,.resource[open]>summary:before { transform:rotate(45deg); }
.chart-title { font-weight:700; font-size:16px; } .chart-meta { color:var(--muted); }
.summary-spacer { flex:1; }
.badges { display:flex; flex-wrap:wrap; gap:6px; align-items:center; }
.badge { display:inline-flex; align-items:center; border:1px solid var(--border); border-radius:999px; padding:2px 8px; font-size:11px; line-height:1.45; white-space:nowrap; background:var(--panel-2); }
.badge.severity-critical { color:var(--critical); border-color:color-mix(in srgb,var(--critical) 40%,var(--border)); }
.badge.severity-high { color:var(--high); border-color:color-mix(in srgb,var(--high) 40%,var(--border)); }
.badge.severity-medium { color:var(--medium); } .badge.severity-low { color:var(--low); } .badge.severity-info { color:var(--info); }
.badge.state-added { color:var(--added); } .badge.state-removed { color:var(--removed); } .badge.state-changed { color:var(--medium); }
.badge.validation-error { color:var(--critical); } .badge.validation-warning { color:var(--medium); } .badge.validation-valid { color:var(--added); }
.chart-body { border-top:1px solid var(--border); padding:14px; background:var(--panel-2); }
.notice { border-left:3px solid var(--medium); background:var(--panel); padding:10px 12px; margin:0 0 10px; border-radius:0 7px 7px 0; }
.notice.error { border-left-color:var(--critical); }
.resource { scroll-margin-top:84px; background:var(--panel); border:1px solid var(--border); border-radius:9px; margin:9px 0; overflow:hidden; }
.resource[hidden],.chart[hidden],.cluster[hidden] { display:none; }
.resource>summary { display:grid; grid-template-columns:auto minmax(180px,1fr) minmax(160px,1.4fr) auto; align-items:center; gap:10px; padding:12px 14px; }
.resource-id { min-width:0; } .resource-id strong { display:block; overflow-wrap:anywhere; } .resource-id span,.resource-summary { color:var(--muted); font-size:12px; }
.resource-body { border-top:1px solid var(--border); padding:16px; }
.resource-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(280px,1fr)); gap:12px; margin-bottom:14px; }
.panel { border:1px solid var(--border); border-radius:8px; padding:12px; min-width:0; } .panel h4 { margin:0 0 8px; font-size:13px; }
.finding { margin:8px 0; } .finding p { margin:2px 0; } .finding-meta { color:var(--muted); font-size:12px; overflow-wrap:anywhere; }
.paths { display:flex; flex-wrap:wrap; gap:6px; } .path { font:11px/1.4 ui-monospace,SFMono-Regular,Consolas,monospace; overflow-wrap:anywhere; }
.diff-block { border:1px solid var(--border); border-radius:8px; margin-top:12px; overflow:hidden; }
.diff-block>summary { cursor:pointer; padding:10px 12px; font-weight:650; background:var(--panel-2); }
.diff { margin:0; padding:12px 0; overflow:auto; background:var(--bg); font-size:12px; line-height:1.5; tab-size:4; }
.diff-line { display:block; min-height:1.5em; padding:0 14px; white-space:pre; }
.diff-line.added { color:var(--added); background:color-mix(in srgb,var(--added) 10%,transparent); }
.diff-line.removed { color:var(--removed); background:color-mix(in srgb,var(--removed) 9%,transparent); }
.diff-line.hunk { color:var(--accent); } .diff-line.meta { color:var(--muted); font-weight:650; }
.diagnostics,.run-info { max-width:1400px; margin:34px auto; }
.diagnostic { background:var(--panel); border:1px solid var(--border); border-radius:8px; margin:8px 0; }
.diagnostic summary { cursor:pointer; padding:10px 12px; } .diagnostic pre { margin:0; padding:12px; border-top:1px solid var(--border); overflow:auto; }
.run-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(220px,1fr)); gap:8px; }
.run-item { background:var(--panel); border:1px solid var(--border); border-radius:8px; padding:10px 12px; }
.run-item span { display:block; color:var(--muted); font-size:11px; text-transform:uppercase; letter-spacing:.05em; } .run-item code { overflow-wrap:anywhere; }
.empty { text-align:center; padding:48px; border:1px dashed var(--border); border-radius:12px; color:var(--muted); }
.footer { max-width:1400px; margin:40px auto 0; color:var(--muted); font-size:12px; border-top:1px solid var(--border); padding-top:14px; }
@media (max-width:850px) { .layout { grid-template-columns:1fr; } .sidebar { position:static; height:auto; border-right:0; border-bottom:1px solid var(--border); } .nav { display:none; } .content { padding-top:22px; } .toolbar { top:6px; } .resource>summary { grid-template-columns:auto minmax(0,1fr) auto; } .resource-summary { display:none; } }
@media print { .sidebar,.toolbar,.sidebar-links { display:none!important; } .layout { display:block; } .content { padding:0; } .chart,.resource { break-inside:avoid; box-shadow:none; } details { display:block; } details>* { display:block!important; } .diff { max-height:none; } }
</style>
</head>
<body>
<div class="layout">
<aside class="sidebar">
  <div class="brand">møbius</div>
  <div class="eyebrow">local artifact report</div>
  <nav class="nav" aria-label="Report navigation">
    {{range .Clusters}}<div class="nav-cluster"><a href="#{{.ID}}">{{.Name}}</a>{{range .Charts}}<a class="nav-chart" href="#{{.ID}}">{{.Name}}</a>{{end}}</div>{{end}}
  </nav>
  <div class="sidebar-links">
    <a href="index.md">Artifact index</a>
    <a href="run-summary.md">Run summary</a>
    <a href="summary.json">Machine summary</a>
  </div>
</aside>
<main class="content">
  <header class="hero">
    <div class="eyebrow">Full report · embedded diffs · works offline</div>
    <h1>møbius Diff Report</h1>
    <p class="lede">Review the highest-risk changes first, filter the resource list, then expand any item for its findings and complete semantic or raw diff.</p>
    <div class="stats">
      <div class="stat"><strong>{{.Stats.Clusters}}</strong><span>clusters</span></div>
      <div class="stat"><strong>{{.Stats.Charts}}</strong><span>charts</span></div>
      <div class="stat"><strong>{{.Stats.Resources}}</strong><span>resources</span></div>
      <div class="stat"><strong>{{.Stats.Added}} / {{.Stats.Removed}} / {{.Stats.Changed}}</strong><span>added / removed / changed</span></div>
      <div class="stat critical"><strong>{{.Stats.Critical}}</strong><span>critical</span></div>
      <div class="stat high"><strong>{{.Stats.High}}</strong><span>high</span></div>
      <div class="stat"><strong>{{.Stats.Invalid}}</strong><span>validation errors</span></div>
      <div class="stat"><strong>{{.Stats.Unvalidated}}</strong><span>unvalidated</span></div>
      <div class="stat"><strong>{{.Stats.Warnings}}</strong><span>chart warnings</span></div>
    </div>
    <div class="toolbar" role="search">
      <input id="search" type="search" placeholder="Search cluster, chart, resource, path, or finding…" aria-label="Search resources">
      <select id="severity" aria-label="Filter by severity"><option value="">All severities</option><option>critical</option><option>high</option><option>medium</option><option>low</option><option>info</option></select>
      <select id="state" aria-label="Filter by state"><option value="">All states</option><option>added</option><option>removed</option><option>changed</option></select>
      <select id="validation" aria-label="Filter by validation"><option value="">All validation</option><option>valid</option><option>warning</option><option>error</option><option>none</option></select>
      <button id="expand" type="button">Expand visible</button><button id="collapse" type="button">Collapse all</button>
      <span class="match-count"><strong id="visible-count">{{.Stats.Resources}}</strong> / {{.Stats.Resources}} resources</span>
    </div>
  </header>

  <section class="report" id="report">
  {{if .Clusters}}
    {{range .Clusters}}<section class="cluster" id="{{.ID}}">
      <div class="cluster-head"><h2>{{.Name}}</h2><div class="counts">{{.Added}} added · {{.Removed}} removed · {{.Changed}} changed</div></div>
      {{range .Charts}}<details class="chart" id="{{.ID}}" data-severity="{{.Severity}}" {{if .Open}}open{{end}}>
        <summary><span class="chart-title">{{.Name}}</span><span class="chart-meta">namespace: {{.Namespace}}</span><span class="summary-spacer"></span><span class="badges"><span class="badge state-added">+{{.Added}}</span><span class="badge state-removed">−{{.Removed}}</span><span class="badge state-changed">~{{.Changed}}</span>{{if .RenderWarning}}<span class="badge validation-error">render warning</span>{{end}}</span></summary>
        <div class="chart-body">
          {{if or .BaselineVersion .CurrentVersion}}<div class="notice">Chart revision: <code>{{if .BaselineVersion}}{{.BaselineVersion}}{{else}}none{{end}}</code> → <code>{{if .CurrentVersion}}{{.CurrentVersion}}{{else}}none{{end}}</code></div>{{end}}
          {{if .RenderWarning}}<div class="notice error"><strong>Render warning</strong><br>{{.RenderWarning}}</div>{{end}}
          {{range .Warnings}}<div class="notice warning"><strong>Warning</strong><br>{{.}}</div>{{end}}
          {{range .Resources}}<details class="resource" id="{{.ID}}" data-search="{{.SearchText}}" data-severity="{{.Severity}}" data-state="{{.State}}" data-validation="{{.ValidationStatus}}" {{if .Open}}open{{end}}>
            <summary><span class="resource-id"><strong>{{.Kind}}/{{.Name}}</strong><span>{{.Namespace}}</span></span><span class="resource-summary">{{.Summary}}</span><span class="badges"><span class="badge state-{{.State}}">{{.State}}</span><span class="badge severity-{{.Severity}}">{{.Severity}}</span><span class="badge validation-{{.ValidationStatus}}">validation: {{.ValidationStatus}}</span></span></summary>
            <div class="resource-body">
              <div class="resource-grid">
                <section class="panel"><h4>Assessment</h4>{{if .Findings}}{{range .Findings}}<div class="finding"><span class="badge severity-{{.Level}}">{{.Level}}</span> <span class="badge">{{.Category}}</span><p>{{.Reason}}</p>{{if .Path}}<div class="finding-meta"><code>{{.Path}}</code></div>{{end}}</div>{{end}}{{else}}<div class="finding-meta">No assessment findings.</div>{{end}}</section>
                <section class="panel"><h4>Validation</h4><div class="badges"><span class="badge validation-{{.ValidationStatus}}">{{.ValidationStatus}}</span><span class="badge">{{.ValidationCoverage}}</span><span class="badge">schema: {{.ValidationSource}}</span></div>{{if .ValidationFindings}}{{range .ValidationFindings}}<div class="finding"><span class="badge validation-{{.Status}}">{{.Status}}</span> <span class="badge">{{.Source}}</span><p>{{.Message}}</p>{{if .Path}}<div class="finding-meta"><code>{{.Path}}</code></div>{{end}}</div>{{end}}{{else}}<div class="finding-meta" style="margin-top:8px">No validation findings.</div>{{end}}</section>
              </div>
              {{if .ChangePaths}}<section class="panel"><h4>Changed paths ({{len .ChangePaths}})</h4><div class="paths">{{range .ChangePaths}}<span class="badge path state-{{.State}}">{{.State}} {{.Path}}</span>{{end}}</div></section>{{end}}
              {{if .Semantic}}<details class="diff-block" open><summary>Semantic diff · {{len .Semantic}} lines</summary><pre class="diff" aria-label="Semantic diff">{{range .Semantic}}<span class="diff-line {{.Class}}">{{.Text}}</span>{{end}}</pre></details>{{end}}
              {{if .Raw}}<details class="diff-block"><summary>Raw unified diff · {{len .Raw}} lines</summary><pre class="diff" aria-label="Raw unified diff">{{range .Raw}}<span class="diff-line {{.Class}}">{{.Text}}</span>{{end}}</pre></details>{{end}}
            </div>
          </details>{{end}}
          {{if not .Resources}}{{if not .RenderWarning}}<div class="empty">No resource changes; this chart contains warnings only.</div>{{end}}{{end}}
        </div>
      </details>{{end}}
    </section>{{end}}
  {{else}}<div class="empty">No effective changes were reported.</div>{{end}}
  </section>

  {{if .Diagnostics}}<section class="diagnostics" id="diagnostics"><h2>Render diagnostics</h2><p class="lede">Error and warning artifacts are embedded here and remain available as individual text files.</p>{{range .Diagnostics}}<details class="diagnostic"><summary><span class="badge validation-{{.Level}}">{{.Level}}</span> <code>{{.Path}}</code></summary><pre>{{.Content}}</pre></details>{{end}}</section>{{end}}

  {{if .Run.Available}}<section class="run-info"><h2>Run information</h2><div class="run-grid"><div class="run-item"><span>Mode</span><code>{{.Run.Mode}}</code></div><div class="run-item"><span>Base ref</span><code>{{.Run.BaseRef}}</code></div><div class="run-item"><span>Merge base</span><code>{{.Run.MergeBaseSHA}}</code></div><div class="run-item"><span>Head</span><code>{{.Run.HeadSHA}}</code></div><div class="run-item"><span>Config</span><code>{{.Run.ConfigSources}}</code></div><div class="run-item"><span>Changed paths</span><code>{{.Run.ChangedPathsCount}}</code></div><div class="run-item"><span>Selected clusters</span><code>{{.Run.SelectedClusters}}</code></div><div class="run-item"><span>Validation</span><code>{{.Run.Validate}}</code></div></div></section>{{end}}
  <footer class="footer">Self-contained report generated by møbius. No network connection or external assets are required.</footer>
</main>
</div>
<script>
(() => {
  const resources = [...document.querySelectorAll('.resource')];
  const search = document.getElementById('search');
  const severity = document.getElementById('severity');
  const state = document.getElementById('state');
  const validation = document.getElementById('validation');
  const count = document.getElementById('visible-count');
  function applyFilters() {
    const query = search.value.trim().toLowerCase();
    let visible = 0;
    resources.forEach(resource => {
      const matches = (!query || resource.dataset.search.includes(query)) && (!severity.value || resource.dataset.severity === severity.value) && (!state.value || resource.dataset.state === state.value) && (!validation.value || resource.dataset.validation === validation.value);
      resource.hidden = !matches;
      if (matches) visible++;
    });
    document.querySelectorAll('.chart').forEach(chart => { chart.hidden = !chart.querySelector('.resource:not([hidden])') && !chart.querySelector('.notice.error,.notice.warning'); });
    document.querySelectorAll('.cluster').forEach(cluster => { cluster.hidden = !cluster.querySelector('.chart:not([hidden])'); });
    count.textContent = visible;
  }
  [search,severity,state,validation].forEach(control => control.addEventListener('input', applyFilters));
  document.getElementById('expand').addEventListener('click', () => document.querySelectorAll('details:not([hidden])').forEach(item => item.open = true));
  document.getElementById('collapse').addEventListener('click', () => document.querySelectorAll('details').forEach(item => item.open = false));
  if (location.hash) { const target = document.querySelector(location.hash); if (target) { let node = target; while (node) { if (node.tagName === 'DETAILS') node.open = true; node = node.parentElement; } } }
})();
</script>
</body>
</html>
`
