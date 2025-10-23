package version

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// Valores padrão (sobrescritos por ldflags ou por build info)
var Version = "0.0.0-dev"
var Commit = ""
var BuildTime = ""

// populateFromBuildInfo tenta preencher Version/Commit/BuildTime usando as informações
// embedadas pelo Go (desde Go 1.18+, buildvcs=on por padrão em module mode).
// Se ldflags já definiu Version/Commit/BuildTime (não-vazios e não-dev), não sobrescrevemos.
func populateFromBuildInfo() {
	// Se já temos uma versão "confiável" de ldflags, não mexe.
	if Version != "" && Version != "0.0.0-dev" {
		return
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok || bi == nil {
		return
	}

	get := func(key string) (string, bool) {
		for _, s := range bi.Settings {
			if s.Key == key {
				return s.Value, true
			}
		}
		return "", false
	}

	// vcs.revision: commit full SHA; usamos curto (7 chars)
	if Commit == "" {
		if rev, ok := get("vcs.revision"); ok && len(rev) >= 7 {
			Commit = rev[:7]
		}
	}

	// vcs.time: RFC3339
	if BuildTime == "" {
		if t, ok := get("vcs.time"); ok && t != "" {
			// Normalizamos no formato YYYY-mm-ddTHH:MM:SSZ
			if ts, err := time.Parse(time.RFC3339, t); err == nil {
				BuildTime = ts.UTC().Format("2006-01-02T15:04:05Z")
			}
		}
	}

	// vcs.modified: "true" se há modificações não commitadas; anexamos "-dirty" à versão
	modified := false
	if m, ok := get("vcs.modified"); ok && (m == "true" || m == "TRUE") {
		modified = true
	}

	// vcs.tag: última tag (quando presente, ex: "v1.2.3")
	// Usamos como Version base. Se não houver, deixamos "0.0.0-dev".
	if tag, ok := get("vcs.tag"); ok && tag != "" {
		// Normaliza removendo prefixo "v"
		tag = strings.TrimPrefix(tag, "v")
		Version = tag
		if modified {
			Version = Version + "-dirty"
		}
	}
}

// init é chamado em package load. Tenta preencher a versão via build info.
func init() {
	populateFromBuildInfo()
}

// CheckLatestVersion verifica se uma versão mais recente está disponível.
func CheckLatestVersion(currentVersion string) {
	// Versões dev não são verificadas
	if strings.HasSuffix(currentVersion, "-dev") {
		return
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/diillson/aws-finops-dashboard-go/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}

	if err := json.Unmarshal(body, &release); err != nil {
		return
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	// Compara versões (heurística simples)
	if latestVersion > currentVersion {
		pterm.Warning.Println(fmt.Sprintf("A new version of AWS FinOps Dashboard is available: %s", latestVersion))
		pterm.Info.Println("Please update using: go install github.com/diillson/aws-finops-dashboard-go@latest")
	}
}

// FormatVersion retorna a versão formatada com commit e build time.
// Ex.: "1.2.3 (commit: abc1234, built at: 2025-10-23T10:20:30Z)"
// Fallbacks: quando não há ldflags, usamos os valores populados via build info.
func FormatVersion() string {
	ver := Version
	if ver == "" {
		ver = "0.0.0-dev"
	}

	commit := Commit
	if commit == "" {
		commit = "development"
	}

	// Quando commit é "development", exibimos "(development)" para clareza
	if commit == "development" && BuildTime == "" {
		return fmt.Sprintf("%s (development)", ver)
	}

	if BuildTime != "" {
		return fmt.Sprintf("%s (commit: %s, built at: %s)", ver, commit, BuildTime)
	}

	return fmt.Sprintf("%s (commit: %s)", ver, commit)
}
