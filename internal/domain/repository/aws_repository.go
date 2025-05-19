package repository

import (
	"context"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
)

// AWSRepository defines the interface for AWS API interactions.
type AWSRepository interface {
	// Profile Operations
	GetAWSProfiles() []string
	GetAccountID(ctx context.Context, profile string) (string, error)
	GetSession(ctx context.Context, profile string) (string, error)

	// Region Operations
	GetAllRegions(ctx context.Context, profile string) ([]string, error)
	GetAccessibleRegions(ctx context.Context, profile string) ([]string, error)

	// Cost Operations
	GetCostData(ctx context.Context, profile string, timeRange *int, tags []string) (entity.CostData, error)
	GetTrendData(ctx context.Context, profile string, tags []string) (map[string]interface{}, error)

	// Budget Operations
	GetBudgets(ctx context.Context, profile string) ([]entity.BudgetInfo, error)

	// EC2 Operations
	GetEC2Summary(ctx context.Context, profile string, regions []string) (entity.EC2Summary, error)
	GetStoppedInstances(ctx context.Context, profile string, regions []string) (entity.StoppedEC2Instances, error)
	GetUnusedVolumes(ctx context.Context, profile string, regions []string) (entity.UnusedVolumes, error)
	GetUnusedEIPs(ctx context.Context, profile string, regions []string) (entity.UnusedEIPs, error)

	// Resource Operations
	GetUntaggedResources(ctx context.Context, profile string, regions []string) (entity.UntaggedResources, error)
}
