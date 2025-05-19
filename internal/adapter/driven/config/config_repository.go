package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// ConfigRepositoryImpl implementa o ConfigRepository.
type ConfigRepositoryImpl struct{}

// NewConfigRepository cria uma nova implementação do ConfigRepository.
func NewConfigRepository() repository.ConfigRepository {
	return &ConfigRepositoryImpl{}
}

// LoadConfigFile carrega um arquivo de configuração TOML, YAML ou JSON.
func (r *ConfigRepositoryImpl) LoadConfigFile(filePath string) (*types.Config, error) {
	fileExtension := filepath.Ext(filePath)
	fileExtension = strings.ToLower(fileExtension)

	// Verifica se o arquivo existe
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("error accessing config file: %w", err)
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", filePath)
	}

	// Lê o arquivo
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var config types.Config

	switch fileExtension {
	case ".toml":
		if err := toml.Unmarshal(fileData, &config); err != nil {
			return nil, fmt.Errorf("error parsing TOML file: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(fileData, &config); err != nil {
			return nil, fmt.Errorf("error parsing YAML file: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(fileData, &config); err != nil {
			return nil, fmt.Errorf("error parsing JSON file: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config file format: %s", fileExtension)
	}

	return &config, nil
}
