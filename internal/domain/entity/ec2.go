package entity

// EC2Summary is a map of instance state names to instance counts.
type EC2Summary map[string]int

// StoppedEC2Instances represents stopped EC2 instances grouped by region.
type StoppedEC2Instances map[string][]string

// UnusedVolumes represents unused EBS volumes grouped by region.
type UnusedVolumes map[string][]string

// UnusedEIPs represents unused Elastic IPs grouped by region.
type UnusedEIPs map[string][]string

// UntaggedResources represents untagged resources grouped by service and region.
type UntaggedResources map[string]map[string][]string

type IdleLoadBalancers map[string][]string
