package types

import "errors"

var (
	ErrNoProfilesFound      = errors.New("no AWS profiles found. Please configure AWS CLI first")
	ErrNoValidProfilesFound = errors.New("none of the specified profiles were found in AWS configuration")
)
