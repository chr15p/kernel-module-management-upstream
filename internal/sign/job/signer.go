package signjob

import (
	"fmt"
	"strings"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/sign"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//go:generate mockgen -source=signer.go -package=signjob -destination=mock_signer.go

type Signer interface {
	MakeJobTemplate(mod kmmv1beta1.Module, signConfig *kmmv1beta1.Sign, targetKernel, previousImage string, containerImage string, pushImage bool) (*batchv1.Job, error)
}

type signer struct {
	helper sign.Helper
	scheme *runtime.Scheme
}

func NewSigner(helper sign.Helper, scheme *runtime.Scheme) Signer {
	return &signer{helper: helper, scheme: scheme}
}

func (m *signer) MakeJobTemplate(mod kmmv1beta1.Module, signConfig *kmmv1beta1.Sign, targetKernel string, previousImage string, targetImage string, pushImage bool) (*batchv1.Job, error) {
	var args []string

	if pushImage {
		args = []string{"-signedimage", targetImage}
	} else {
		args = append(args, "-no-push")
	}

	if previousImage != "" {
		args = append(args, "-unsignedimage", previousImage)
	} else if signConfig.UnsignedImage != "" {
		args = append(args, "-unsignedimage", signConfig.UnsignedImage)
	} else {
		return nil, fmt.Errorf("no image to sign given")
	}
	args = append(args, "-key", "/signingkey/key.priv")
	args = append(args, "-cert", "/signingcert/public.der")

	if len(signConfig.FilesToSign) > 0 {
		args = append(args, "-filestosign", strings.Join(signConfig.FilesToSign, ":"))
	}

	volumes := []v1.Volume{}
	volumeMounts := []v1.VolumeMount{}
	volumes = append(volumes, m.makeImageSigningSecretVolume(signConfig.KeySecret, "key", "key.priv"))
	volumes = append(volumes, m.makeImageSigningSecretVolume(signConfig.CertSecret, "cert", "public.der"))

	volumeMounts = append(volumeMounts, m.makeImageSecretVolumeMount(signConfig.CertSecret, "/signingcert"))
	volumeMounts = append(volumeMounts, m.makeImageSecretVolumeMount(signConfig.KeySecret, "/signingkey"))

	if mod.Spec.ImageRepoSecret != nil {
		args = append(args, "-pullsecret", "/docker_config/config.json")
		volumes = append(volumes, m.makeImageSigningSecretVolume(mod.Spec.ImageRepoSecret, v1.DockerConfigJsonKey, "config.json"))
		volumeMounts = append(volumeMounts, m.makeImageSecretVolumeMount(mod.Spec.ImageRepoSecret, "/docker_config"))
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: mod.Name + "-sign-",
			Namespace:    mod.Namespace,
			Labels:       labels(mod, targetKernel),
		},
		Spec: batchv1.JobSpec{
			Completions: pointer.Int32(1),
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:         "signimage",
							Image:        "quay.io/chrisp262/kmod-signer:latest",
							Args:         args,
							VolumeMounts: volumeMounts,
						},
					},
					RestartPolicy: v1.RestartPolicyOnFailure,
					Volumes:       volumes,
					NodeSelector:  mod.Spec.Selector,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(&mod, job, m.scheme); err != nil {
		return nil, fmt.Errorf("could not set the owner reference: %v", err)
	}

	return job, nil
}

func (m *signer) makeImageSigningSecretVolume(secretRef *v1.LocalObjectReference, key string, path string) v1.Volume {
	if secretRef == nil {
		return v1.Volume{}
	}

	return v1.Volume{
		Name: m.volumeNameFromSecretRef(*secretRef),
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secretRef.Name,
				Items: []v1.KeyToPath{
					{
						Key:  key,
						Path: path,
					},
				},
			},
		},
	}
}

func (m *signer) makeImageSecretVolumeMount(secretRef *v1.LocalObjectReference, mountPath string) v1.VolumeMount {

	if secretRef == nil {
		return v1.VolumeMount{}
	}

	return v1.VolumeMount{
		Name:      m.volumeNameFromSecretRef(*secretRef),
		ReadOnly:  true,
		MountPath: mountPath,
	}
}

func (m *signer) volumeNameFromSecretRef(ref v1.LocalObjectReference) string {
	return "secret-" + ref.Name
}
