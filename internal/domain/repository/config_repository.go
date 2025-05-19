package repository

import (
	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
)

// ConfigRepository defines the interface for loading configuration files.
type ConfigRepository interface {
	LoadConfigFile(filePath string) (*types.Config, error)
}
