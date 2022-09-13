package build

import (
	"github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
)

//go:generate mockgen -source=helper.go -package=build -destination=mock_helper.go

type Helper interface {
	ApplyBuildArgOverrides(args []v1beta1.BuildArg, overrides ...v1beta1.BuildArg) []v1beta1.BuildArg
	GetRelevantBuild(mod kmmv1beta1.Module, km kmmv1beta1.KernelMapping) *kmmv1beta1.Build
	GetRelevantSign(mod kmmv1beta1.Module, km kmmv1beta1.KernelMapping) *kmmv1beta1.Sign
}

type helper struct{}

func NewHelper() Helper {
	return &helper{}
}

func (m *helper) ApplyBuildArgOverrides(args []v1beta1.BuildArg, overrides ...v1beta1.BuildArg) []v1beta1.BuildArg {
	overridesMap := make(map[string]v1beta1.BuildArg, len(overrides))

	for _, o := range overrides {
		overridesMap[o.Name] = o
	}

	unusedOverrides := sets.StringKeySet(overridesMap)

	for i := 0; i < len(args); i++ {
		argName := args[i].Name

		if o, ok := overridesMap[argName]; ok {
			args[i] = o
			unusedOverrides.Delete(argName)
		}
	}

	for _, overrideName := range unusedOverrides.List() {
		args = append(args, overridesMap[overrideName])
	}

	return args
}

func (m *helper) GetRelevantBuild(mod kmmv1beta1.Module, km kmmv1beta1.KernelMapping) *kmmv1beta1.Build {
	if mod.Spec.ModuleLoader.Container.Build == nil {
		// km.Build cannot be nil in case mod.Build is nil, checked above
		return km.Build.DeepCopy()
	}

	if km.Build == nil {
		return mod.Spec.ModuleLoader.Container.Build.DeepCopy()
	}

	buildConfig := mod.Spec.ModuleLoader.Container.Build.DeepCopy()
	if km.Build.Dockerfile != "" {
		buildConfig.Dockerfile = km.Build.Dockerfile
	}

	buildConfig.BuildArgs = m.ApplyBuildArgOverrides(buildConfig.BuildArgs, km.Build.BuildArgs...)

	// [TODO] once MGMT-10832 is consolidated, this code must be revisited. We will decide which
	// secret and how to use, and if we need to take care of repeated secrets names
	buildConfig.Secrets = append(buildConfig.Secrets, km.Build.Secrets...)
	return buildConfig
}

func (m *helper) GetRelevantSign(mod kmmv1beta1.Module, km kmmv1beta1.KernelMapping) *kmmv1beta1.Sign {
	if mod.Spec.ModuleLoader.Container.Sign == nil {
		// km.Build cannot be nil in case mod.Build is nil, checked above
		return km.Sign.DeepCopy()
	}

	if km.Sign == nil {
		return mod.Spec.ModuleLoader.Container.Sign.DeepCopy()
	}

	signConfig := mod.Spec.ModuleLoader.Container.Sign.DeepCopy()

	// this would be better done with reflection but this will work for the moment
	// while we get all the defaults nailed down
	if km.Sign.UnsignedImage != "" {
		// this should default to whatevcer the build produces
		signConfig.UnsignedImage = km.Sign.UnsignedImage
	}

	if km.Sign.SignedImage != "" {
		signConfig.SignedImage = km.Sign.SignedImage
	} else if (signConfig.SignedImage == ""){
		signConfig.SignedImage = mod.Spec.ModuleLoader.Container.ContainerImage
	}

	if km.Sign.KeySecret != nil {
		signConfig.KeySecret = km.Sign.KeySecret
	}

	if km.Sign.CertSecret != nil {
		signConfig.CertSecret = km.Sign.CertSecret
	}

	if len(km.Sign.FilesToSign) > 0 {
		signConfig.FilesToSign = km.Sign.FilesToSign
	}

	if km.Sign.Pull != nil {
		signConfig.Pull = km.Sign.Pull
	}

	if km.Sign.Push != nil {
		signConfig.Push = km.Sign.Push
	}

	return signConfig
}

