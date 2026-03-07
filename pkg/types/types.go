package types

import (
	"time"
)

type Package struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Module  string `json:"module"`
}

type Dependency struct {
	Package  Package `json:"package"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	FullName string  `json:"full_name"`
}

type Function struct {
	Name           string       `json:"name"`
	FullName       string       `json:"full_name"`
	Package        string       `json:"package"`
	File           string       `json:"file"`
	StartLine      int          `json:"start_line"`
	EndLine        int          `json:"end_line"`
	IsExported     bool         `json:"is_exported"`
	IsMain         bool         `json:"is_main"`
	ASTHash        string       `json:"ast_hash"`
	TransitiveHash string       `json:"transitive_hash"`
	Deps           []Dependency `json:"deps"`
}

type CallGraph struct {
	Nodes         map[string]Function `json:"nodes"`
	ReverseIndex  map[string][]string `json:"reverse_index"`
	FunctionOwner map[string]string   `json:"function_owner"`
}

type HashInfo struct {
	ASTHash        string   `json:"ast_hash"`
	TransitiveHash string   `json:"transitive_hash"`
	DepsHash       string   `json:"deps_hash"`
	ExternalDeps   []string `json:"external_deps"`
}

type Baseline struct {
	Version        string              `json:"version"`
	GeneratedAt    time.Time           `json:"generated_at"`
	Commit         string              `json:"commit"`
	GoVersion      string              `json:"go_version"`
	ModulePath     string              `json:"module_path"`
	Graph          CallGraph           `json:"graph"`
	FunctionHashes map[string]HashInfo `json:"function_hashes"`
	ExternalDeps   map[string]string   `json:"external_deps"`
	SourceHashes   map[string]string   `json:"source_hashes"`
}

type Change struct {
	Function string `json:"function"`
	Type     string `json:"type"`
	Reason   string `json:"reason"`
	OldHash  string `json:"old_hash,omitempty"`
	NewHash  string `json:"new_hash,omitempty"`
	Package  string `json:"package,omitempty"`
	OldVer   string `json:"old_version,omitempty"`
	NewVer   string `json:"new_version,omitempty"`
}

type Impact struct {
	AffectedFunctions map[string][]string `json:"affected_functions"`
	AffectReasons     map[string][]string `json:"affect_reasons"`
	ServicesToBuild   []string            `json:"services_to_build"`
	Changes           []Change            `json:"changes"`
}

type DebugInfo struct {
	FilesParsed    int   `json:"files_parsed"`
	FunctionsFound int   `json:"functions_found"`
	AnalysisTimeMs int64 `json:"analysis_time_ms"`
	CacheHit       bool  `json:"cache_hit"`
}

type Result struct {
	Timestamp        time.Time  `json:"timestamp"`
	PreviousCommit   string     `json:"previous_commit"`
	CurrentCommit    string     `json:"current_commit"`
	PreviousBaseline string     `json:"previous_baseline"`
	HasChanges       bool       `json:"has_changes"`
	Changes          []Change   `json:"changes"`
	Impact           Impact     `json:"impact"`
	Debug            *DebugInfo `json:"debug,omitempty"`
}

type SourceFileInfo struct {
	File   string `json:"file"`
	Hash   string `json:"hash"`
	Parsed bool   `json:"parsed"`
}

type ImpactIndex struct {
	FunctionToServices map[string][]string `json:"function_to_services"`
	ServiceToModules   map[string][]string `json:"service_to_modules"`
}
