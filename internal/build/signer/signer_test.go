package signer

import (
	"strings"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/build"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	//"golang.org/x/exp/slices"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("MakeJob", func() {

	const (
		unsignedImage  = "my.registry/my/image"
		signedImage    = "my.registry/my/image-signed"
		filesToSign    = "/modules/simple-kmod.ko:/modules/simple-procfs-kmod.ko"
		//fileToSignArr  = {"/modules/simple-kmod.ko","/modules/simple-procfs-kmod.ko"}
		dockerfile     = "FROM test"
		kernelVersion  = "1.2.3"
		moduleName     = "module-name"
		namespace      = "some-namespace"
	)

	var (
		ctrl *gomock.Controller
		m    Signer
		mh   *build.MockHelper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mh = build.NewMockHelper(ctrl)
		m = NewSigner(mh, scheme)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	mod := kmmv1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{
			Name:      moduleName,
			Namespace: namespace,
		},
	}


	DescribeTable("should set fields correctly", func(imagePullSecret *v1.LocalObjectReference) {
		nodeSelector := map[string]string{"arch": "x64"}

		km := kmmv1beta1.KernelMapping{
			Sign: &kmmv1beta1.Sign{
            			UnsignedImage: unsignedImage,
            			SignedImage: signedImage,
            			KeySecret: &v1.LocalObjectReference{Name: "securebootkey"},
            			CertSecret: &v1.LocalObjectReference{Name: "securebootcert"},
            			FilesToSign: strings.Split(filesToSign, ","),
			},
			ContainerImage: signedImage,
		}

		expected := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: mod.Name + "-sign-",
				Namespace:    namespace,
				Labels: map[string]string{
					constants.ModuleNameLabel:    moduleName,
					constants.TargetKernelTarget: kernelVersion,
					constants.BuildStage: "sign",
				},

				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "kmm.sigs.k8s.io/v1beta1",
						Kind:               "Module",
						Name:               moduleName,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
			Spec: batchv1.JobSpec{
				Completions: pointer.Int32(1),
				Template: v1.PodTemplateSpec{
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:         "signimage",
								Image:        "quay.io/chrisp262/kmod-signer:latest",
								Args: []string{
									"-signedimage", signedImage,
									"-unsignedimage", unsignedImage,
									"-pullsecret", "/docker_config/config.json",
									"-key", "/signingkey/key.priv",
									"-cert", "/signingcert/public.der",
									"-filestosign", filesToSign,
								},
								//RestartPolicy: v1.RestartPolicyOnFailure,
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      "secret-securebootcert",
										ReadOnly:  true,
										MountPath: "/signingcert",
									},
									{
										Name:      "secret-securebootkey",
										ReadOnly:  true,
										MountPath: "/signingkey",
									},
								},
							},
						},
						NodeSelector:  nodeSelector,
						RestartPolicy: v1.RestartPolicyOnFailure,

						Volumes: []v1.Volume{
							{
								Name: "secret-securebootkey",
								VolumeSource: v1.VolumeSource{
									Secret: &v1.SecretVolumeSource{
										SecretName: "securebootkey",
										Items: []v1.KeyToPath{
											{
											Key:  "key",
											Path: "key.priv",
											},
										},
									},
								},
							},
							{
								Name: "secret-securebootcert",
								VolumeSource: v1.VolumeSource{
									Secret: &v1.SecretVolumeSource{
										SecretName: "securebootcert",
										Items: []v1.KeyToPath{
											{
											Key:  "cert",
											Path: "public.der",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}	
		if imagePullSecret != nil {
			mod.Spec.ImageRepoSecret = imagePullSecret

			expected.Spec.Template.Spec.Containers[0].VolumeMounts =
				append(expected.Spec.Template.Spec.Containers[0].VolumeMounts,
					v1.VolumeMount{
						Name:      "secret-pull-push-secret",
						ReadOnly:  true,
						MountPath: "/docker_config",
					},
				)

			expected.Spec.Template.Spec.Volumes =
				append(expected.Spec.Template.Spec.Volumes,
					v1.Volume{
						Name: "secret-pull-push-secret",
						VolumeSource: v1.VolumeSource{
							Secret: &v1.SecretVolumeSource{
								SecretName: "pull-push-secret",
								Items: []v1.KeyToPath{
									{
										Key:  v1.DockerConfigJsonKey,
										Path: "config.json",
									},
								},
							},
						},
					},
				)
		}

		mod := mod.DeepCopy()
		mod.Spec.Selector = nodeSelector

		actual, err := m.MakeJob(*mod, km.Sign, kernelVersion, true)
		Expect(err).NotTo(HaveOccurred())

		Expect(
			cmp.Diff(expected, actual),
		).To(
			BeEmpty(),
		)
	},
		Entry(
			"no secrets at all",
			nil,
		),
		Entry(
			"only imagePullSecrets",
			&v1.LocalObjectReference{Name: "pull-push-secret"},
		),
	)

	DescribeTable("should set correct kmod-signer flags", func(filelist []string, pushFlag bool) {
/*
		km := kmmv1beta1.KernelMapping{
			Sign: &kmmv1beta1.Sign{
				BuildArgs:  buildArgs,
				Dockerfile: dockerfile,
			},
			ContainerImage: containerImage,
		}
*/
		signConfig := &kmmv1beta1.Sign{
            			UnsignedImage: unsignedImage,
            			SignedImage: signedImage,
            			KeySecret: &v1.LocalObjectReference{Name: "securebootkey"},
            			CertSecret: &v1.LocalObjectReference{Name: "securebootcert"},
            			FilesToSign: filelist,
			}

		//mh.EXPECT().ApplyBuildArgOverrides(nil, kmmv1beta1.BuildArg{Name: "KERNEL_VERSION", Value: kernelVersion})

		actual, err := m.MakeJob(mod, signConfig, kernelVersion, pushFlag)
		Expect(err).NotTo(HaveOccurred())
		Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-unsignedimage"))
		Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-pullsecret"))
		Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-key"))
		Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-cert"))
		if pushFlag {
			Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-signedimage"))
		} else {
			Expect(actual.Spec.Template.Spec.Containers[0].Args).To(ContainElement("-no-push"))
		}

	},
		Entry(
			"filelist and push",
			[]string{"simple-kmod","complicated-kmod"},
			true,
		),
		Entry(
			"filelist and no push",
			[]string{"simple-kmod","complicated-kmod"},
			false,
		),
		Entry(
			"all kmods and push",
			[]string{},
			true,
		),
		Entry(
			"all kkmods and dont push",
			[]string{},
			false,
		),
	)

/*
	Describe("should override kaniko image tag", func() {
		It("use a custom given tag", func() {
			const customTag = "some-tag"

			km := kmmv1beta1.KernelMapping{
				Build: &kmmv1beta1.Build{
					BuildArgs:    buildArgs,
					Dockerfile:   dockerfile,
					KanikoParams: &kmmv1beta1.KanikoParams{Tag: customTag},
				},
				ContainerImage: containerImage,
			}

			override := kmmv1beta1.BuildArg{Name: "KERNEL_VERSION", Value: kernelVersion}
			mh.EXPECT().ApplyBuildArgOverrides(buildArgs, override)

			actual, err := m.MakeJob(mod, km.Build, kernelVersion, km.ContainerImage, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(actual.Spec.Template.Spec.Containers[0].Image).To(Equal("gcr.io/kaniko-project/executor:" + customTag))
		})
	})
	*/
})
