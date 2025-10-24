package repository

import (
	"context"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
)

type AWSRepository interface {
	// Profile Operations
	GetAWSProfiles() []string
	GetAccountID(ctx context.Context, profile string) (string, error)
	GetSession(ctx context.Context, profile string) (string, error)

	// Region Operations
	GetAllRegions(ctx context.Context, profile string) ([]string, error)
	GetAccessibleRegions(ctx context.Context, profile string) ([]string, error)

	// Cost Operations
	GetCostData(ctx context.Context, profile string, timeRange *int, tags []string, breakdown bool) (entity.CostData, error)
	GetTrendData(ctx context.Context, profile string, tags []string) (map[string]interface{}, error)

	// Budget Operations
	GetBudgets(ctx context.Context, profile string) ([]entity.BudgetInfo, error)

	// EC2 & Resource Operations
	GetEC2Summary(ctx context.Context, profile string, regions []string) (entity.EC2Summary, error)
	GetStoppedInstances(ctx context.Context, profile string, regions []string) (entity.StoppedEC2Instances, error)
	GetUnusedVolumes(ctx context.Context, profile string, regions []string) (entity.UnusedVolumes, error)
	GetUnusedEIPs(ctx context.Context, profile string, regions []string) (entity.UnusedEIPs, error)
	GetUntaggedResources(ctx context.Context, profile string, regions []string) (entity.UntaggedResources, error)
	GetIdleLoadBalancers(ctx context.Context, profile string, regions []string) (entity.IdleLoadBalancers, error)
	GetNatGatewayCost(ctx context.Context, profile string, timeRange *int, tags []string) ([]entity.NatGatewayCost, error)
	GetUnusedVpcEndpoints(ctx context.Context, profile string, regions []string) (entity.UnusedVpcEndpoints, error)

	// Data Transfer
	GetDataTransferBreakdown(ctx context.Context, profile string, timeRange *int, tags []string) (entity.DataTransferReport, error)

	// CloudWatch Logs
	GetCloudWatchLogGroups(ctx context.Context, profile string, regions []string) ([]entity.CloudWatchLogGroupInfo, error)

	// S3 Lifecycle
	GetS3LifecycleStatus(ctx context.Context, profile string) ([]entity.S3BucketLifecycleStatus, error)

	// Savings Plans / Reserved Instances (Coverage & Utilization)
	GetSavingsPlansSummary(ctx context.Context, profile string, timeRange *int, tags []string) (entity.SPSummary, error)
	GetReservationSummary(ctx context.Context, profile string, timeRange *int, tags []string) (entity.RISummary, error)
}
