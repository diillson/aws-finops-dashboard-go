package version

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"encoding/json"
	"github.com/pterm/pterm"
)

// Version é a versão atual do AWS FinOps Dashboard
const Version = "1.0.0"

// Commit é o commit do git que gerou este build (definido durante a compilação)
var Commit = "development"

// BuildTime é o momento em que a build foi gerada (definido durante a compilação)
var BuildTime = ""

// CheckLatestVersion verifica se uma versão mais recente está disponível.
func CheckLatestVersion(currentVersion string) {
	// Versões de desenvolvimento não são verificadas
	if strings.HasSuffix(currentVersion, "-dev") {
		return
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

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

	// Compara versões (simples)
	if latestVersion > currentVersion {
		pterm.Warning.Println(fmt.Sprintf("A new version of AWS FinOps Dashboard is available: %s", latestVersion))
		pterm.Info.Println("Please update using: go install github.com/diillson/aws-finops-dashboard-go@latest")
	}
}

// FormatVersion retorna a versão formatada com informações de commit e build time.
func FormatVersion() string {
	if Commit == "development" {
		return fmt.Sprintf("%s (development)", Version)
	}

	if BuildTime != "" {
		return fmt.Sprintf("%s (commit: %s, built at: %s)", Version, Commit, BuildTime)
	}

	return fmt.Sprintf("%s (commit: %s)", Version, Commit)
}
