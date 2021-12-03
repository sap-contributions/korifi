package actions

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name Client sigs.k8s.io/controller-runtime/pkg/client.Client

//counterfeiter:generate -o fake -fake-name CFProcessRepository . CFProcessRepository
type CFProcessRepository interface {
	FetchProcess(context.Context, client.Client, string) (repositories.ProcessRecord, error)
	FetchProcessList(context.Context, client.Client, repositories.FetchProcessListMessage) ([]repositories.ProcessRecord, error)
	ScaleProcess(context.Context, client.Client, repositories.ProcessScaleMessage) (repositories.ProcessRecord, error)
	CreateProcess(context.Context, client.Client, repositories.ProcessCreateMessage) error
	FetchProcessByAppTypeAndSpace(context.Context, client.Client, string, string, string) (repositories.ProcessRecord, error)
	PatchProcess(context.Context, client.Client, repositories.ProcessPatchMessage) error
}

//counterfeiter:generate -o fake -fake-name CFAppRepository . CFAppRepository
type CFAppRepository interface {
	FetchApp(context.Context, client.Client, string) (repositories.AppRecord, error)
	FetchAppByNameAndSpace(context.Context, client.Client, string, string) (repositories.AppRecord, error)
	FetchNamespace(context.Context, client.Client, string) (repositories.SpaceRecord, error)
	CreateOrPatchAppEnvVars(context.Context, client.Client, repositories.CreateOrPatchAppEnvVarsMessage) (repositories.AppEnvVarsRecord, error)
	CreateApp(context.Context, client.Client, repositories.AppCreateMessage) (repositories.AppRecord, error)
}

//counterfeiter:generate -o fake -fake-name PodRepository . PodRepository
type PodRepository interface {
	FetchPodStatsByAppGUID(ctx context.Context, k8sClient client.Client, message repositories.FetchPodStatsMessage) ([]repositories.PodStatsRecord, error)
}
