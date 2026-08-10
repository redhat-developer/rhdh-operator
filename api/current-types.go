package api

import (
	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"
)

// Type aliases to decouple application code from specific API versions.
// When upgrading API versions, only the import in this file needs to be updated.
//
// All application code should import from this package instead of importing
// versioned API packages directly. For example:
//
//	import "github.com/redhat-developer/rhdh-operator/api"
//
// Then use api.Backstage, api.BackstageSpec, etc.

type (
	// Core types
	Backstage       = bsv1.Backstage
	BackstageSpec   = bsv1.BackstageSpec
	BackstageStatus = bsv1.BackstageStatus
	BackstageList   = bsv1.BackstageList

	// Condition types
	BackstageConditionType   = bsv1.BackstageConditionType
	BackstageConditionReason = bsv1.BackstageConditionReason

	// Spec components
	Flavour             = bsv1.Flavour
	Application         = bsv1.Application
	Database            = bsv1.Database
	AppConfig           = bsv1.AppConfig
	ExtraEnvs           = bsv1.ExtraEnvs
	ExtraFiles          = bsv1.ExtraFiles
	Route               = bsv1.Route
	RuntimeConfig       = bsv1.RuntimeConfig
	BackstageDeployment = bsv1.BackstageDeployment
	Monitoring          = bsv1.Monitoring

	// Reference types
	EnvObjectRef  = bsv1.EnvObjectRef
	FileObjectRef = bsv1.FileObjectRef
	PvcRef        = bsv1.PvcRef
	Env           = bsv1.Env

	// Other types
	TLS = bsv1.TLS
)

// Condition constants
const (
	// Condition types
	BackstageConditionTypeDeployed BackstageConditionType = bsv1.BackstageConditionTypeDeployed
	BackstageConditionTypeRuntime  BackstageConditionType = bsv1.BackstageConditionTypeRuntime
	BackstageConditionTypeConfig   BackstageConditionType = bsv1.BackstageConditionTypeConfig

	// Deployed condition reasons
	BackstageConditionReasonDeployed   BackstageConditionReason = bsv1.BackstageConditionReasonDeployed
	BackstageConditionReasonFailed     BackstageConditionReason = bsv1.BackstageConditionReasonFailed
	BackstageConditionReasonInProgress BackstageConditionReason = bsv1.BackstageConditionReasonInProgress

	// Runtime condition reasons
	BackstageConditionReasonRunning         BackstageConditionReason = bsv1.BackstageConditionReasonRunning
	BackstageConditionReasonContainerFailed BackstageConditionReason = bsv1.BackstageConditionReasonContainerFailed
	BackstageConditionReasonPending         BackstageConditionReason = bsv1.BackstageConditionReasonPending

	// Config condition reasons
	BackstageConditionReasonInvalid BackstageConditionReason = bsv1.BackstageConditionReasonInvalid
	BackstageConditionReasonIdled      BackstageConditionReason = bsv1.BackstageConditionReasonIdled
)

// AddToScheme adds the current API version's types to the scheme.
// This delegates to the underlying versioned API's AddToScheme.
var AddToScheme = bsv1.AddToScheme
