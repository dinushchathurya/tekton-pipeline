/*
Copyright 2022 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tektoncd/pipeline/pkg/apis/config"
	pipelineErrors "github.com/tektoncd/pipeline/pkg/apis/pipeline/errors"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/pod"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/test/diff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

func TestPipelineRunStatusConditions(t *testing.T) {
	p := &v1.PipelineRun{}
	foo := &apis.Condition{
		Type:   "Foo",
		Status: "True",
	}
	bar := &apis.Condition{
		Type:   "Bar",
		Status: "True",
	}

	var ignoreVolatileTime = cmp.Comparer(func(_, _ apis.VolatileTime) bool {
		return true
	})

	// Add a new condition.
	p.Status.SetCondition(foo)

	fooStatus := p.Status.GetCondition(foo.Type)
	if d := cmp.Diff(foo, fooStatus, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected pipeline run condition type; diff %v", diff.PrintWantGot(d))
	}

	// Add a second condition.
	p.Status.SetCondition(bar)

	barStatus := p.Status.GetCondition(bar.Type)

	if d := cmp.Diff(bar, barStatus, ignoreVolatileTime); d != "" {
		t.Fatalf("Unexpected pipeline run condition type; diff %s", diff.PrintWantGot(d))
	}
}

func TestInitializePipelineRunConditions(t *testing.T) {
	p := &v1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-name",
			Namespace: "test-ns",
		},
	}
	p.Status.InitializeConditions(testClock)

	if p.Status.StartTime.IsZero() {
		t.Fatalf("PipelineRun StartTime not initialized correctly")
	}

	condition := p.Status.GetCondition(apis.ConditionSucceeded)
	if condition.Reason != v1.PipelineRunReasonStarted.String() {
		t.Fatalf("PipelineRun initialize reason should be %s, got %s instead", v1.PipelineRunReasonStarted.String(), condition.Reason)
	}

	// Change the reason before we initialize again
	p.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionUnknown,
		Reason:  "not just started",
		Message: "hello",
	})

	p.Status.InitializeConditions(testClock)

	newCondition := p.Status.GetCondition(apis.ConditionSucceeded)
	if newCondition.Reason != "not just started" {
		t.Fatalf("PipelineRun initialize reset the condition reason to %s", newCondition.Reason)
	}
}

func TestPipelineRunIsDone(t *testing.T) {
	pr := &v1.PipelineRun{}
	foo := &apis.Condition{
		Type:   apis.ConditionSucceeded,
		Status: corev1.ConditionFalse,
	}
	pr.Status.SetCondition(foo)
	if !pr.IsDone() {
		t.Fatal("Expected pipelinerun status to be done")
	}
}

func TestPipelineRunIsCancelled(t *testing.T) {
	pr := &v1.PipelineRun{
		Spec: v1.PipelineRunSpec{
			Status: v1.PipelineRunSpecStatusCancelled,
		},
	}
	if !pr.IsCancelled() {
		t.Fatal("Expected pipelinerun status to be cancelled")
	}
}

func TestPipelineRunIsGracefullyCancelled(t *testing.T) {
	pr := &v1.PipelineRun{
		Spec: v1.PipelineRunSpec{
			Status: v1.PipelineRunSpecStatusCancelledRunFinally,
		},
	}
	if !pr.IsGracefullyCancelled() {
		t.Fatal("Expected pipelinerun status to be gracefully cancelled")
	}
}

func TestPipelineRunIsGracefullyStopped(t *testing.T) {
	pr := &v1.PipelineRun{
		Spec: v1.PipelineRunSpec{
			Status: v1.PipelineRunSpecStatusStoppedRunFinally,
		},
	}
	if !pr.IsGracefullyStopped() {
		t.Fatal("Expected pipelinerun status to be gracefully stopped")
	}
}

func TestPipelineRunHasVolumeClaimTemplate(t *testing.T) {
	pr := &v1.PipelineRun{
		Spec: v1.PipelineRunSpec{
			Workspaces: []v1.WorkspaceBinding{{
				Name: "my-workspace",
				VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc",
					},
					Spec: corev1.PersistentVolumeClaimSpec{},
				},
			}},
		},
	}
	if !pr.HasVolumeClaimTemplate() {
		t.Fatal("Expected pipelinerun to have a volumeClaimTemplate workspace")
	}
}

func TestGetNamespacedName(t *testing.T) {
	pr := &v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: "prunname"}}
	n := pr.GetNamespacedName()
	expected := "foo/prunname"
	if n.String() != expected {
		t.Fatalf("Expected name to be %s but got %s", expected, n.String())
	}
}

func TestPipelineRunHasStarted(t *testing.T) {
	params := []struct {
		name          string
		prStatus      v1.PipelineRunStatus
		expectedValue bool
	}{{
		name:          "prWithNoStartTime",
		prStatus:      v1.PipelineRunStatus{},
		expectedValue: false,
	}, {
		name: "prWithStartTime",
		prStatus: v1.PipelineRunStatus{
			PipelineRunStatusFields: v1.PipelineRunStatusFields{
				StartTime: &metav1.Time{Time: now},
			},
		},
		expectedValue: true,
	}, {
		name: "prWithZeroStartTime",
		prStatus: v1.PipelineRunStatus{
			PipelineRunStatusFields: v1.PipelineRunStatusFields{
				StartTime: &metav1.Time{},
			},
		},
		expectedValue: false,
	}}
	for _, tc := range params {
		t.Run(tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prunname",
					Namespace: "testns",
				},
				Status: tc.prStatus,
			}
			if pr.HasStarted() != tc.expectedValue {
				t.Fatalf("Expected pipelinerun HasStarted() to return %t but got %t", tc.expectedValue, pr.HasStarted())
			}
		})
	}
}

func TestPipelineRunIsTimeoutConditionSet(t *testing.T) {
	tcs := []struct {
		name      string
		condition apis.Condition
		want      bool
	}{{
		name: "should return true when reason is timeout",
		condition: apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: corev1.ConditionFalse,
			Reason: v1.PipelineRunReasonTimedOut.String(),
		},
		want: true,
	}, {
		name: "should return false if status is not false",
		condition: apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: corev1.ConditionUnknown,
			Reason: v1.PipelineRunReasonTimedOut.String(),
		},
		want: false,
	}, {
		name: "should return false if the reason is not timeout",
		condition: apis.Condition{
			Type:   apis.ConditionSucceeded,
			Status: corev1.ConditionFalse,
			Reason: v1.PipelineRunReasonFailed.String(),
		},
		want: false,
	}}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline-run"},
				Status: v1.PipelineRunStatus{
					Status: duckv1.Status{
						Conditions: duckv1.Conditions{tc.condition},
					},
				},
			}
			if got := pr.IsTimeoutConditionSet(); got != tc.want {
				t.Errorf("pr.IsTimeoutConditionSet() (-want, +got):\n- %t\n+ %t", tc.want, got)
			}
		})
	}
}

func TestPipelineRunSetTimeoutCondition(t *testing.T) {
	ctx := config.ToContext(t.Context(), &config.Config{
		Defaults: &config.Defaults{
			DefaultTimeoutMinutes: 120,
		},
	})

	tcs := []struct {
		name        string
		pipelineRun *v1.PipelineRun
		want        *apis.Condition
	}{{
		name:        "set condition to default timeout",
		pipelineRun: &v1.PipelineRun{ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline-run"}},
		want: &apis.Condition{
			Type:    "Succeeded",
			Status:  "False",
			Reason:  "PipelineRunTimeout",
			Message: `PipelineRun "test-pipeline-run" failed to finish within "2h0m0s"`,
		},
	}, {
		name: "set condition to spec.timeouts.pipeline value",
		pipelineRun: &v1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pipeline-run"},
			Spec: v1.PipelineRunSpec{
				Timeouts: &v1.TimeoutFields{
					Pipeline: &metav1.Duration{Duration: time.Hour},
				},
			},
		},
		want: &apis.Condition{
			Type:    "Succeeded",
			Status:  "False",
			Reason:  "PipelineRunTimeout",
			Message: `PipelineRun "test-pipeline-run" failed to finish within "1h0m0s"`,
		},
	}}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			tc.pipelineRun.SetTimeoutCondition(ctx)

			got := tc.pipelineRun.Status.GetCondition(apis.ConditionSucceeded)
			if d := cmp.Diff(tc.want, got, cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime")); d != "" {
				t.Errorf("Unexpected PipelineRun condition: %v", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineRunHasTimedOutForALongTime(t *testing.T) {
	tcs := []struct {
		name      string
		timeout   time.Duration
		starttime time.Time
		expected  bool
	}{{
		name:      "has timed out for a long time",
		timeout:   1 * time.Hour,
		starttime: now.Add(-2 * time.Hour),
		expected:  true,
	}, {
		name:      "has timed out for not a long time",
		timeout:   1 * time.Hour,
		starttime: now.Add(-90 * time.Minute),
		expected:  false,
	}, {
		name:      "has not timed out",
		timeout:   1 * time.Hour,
		starttime: now.Add(-30 * time.Minute),
		expected:  false,
	}, {
		name:      "has no timeout specified",
		timeout:   0 * time.Second,
		starttime: now.Add(-24 * time.Hour),
		expected:  false,
	}}

	for _, tc := range tcs {
		t.Run("pipeline.timeouts.pipeline "+tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: v1.PipelineRunSpec{
					Timeouts: &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: tc.timeout}},
				},
				Status: v1.PipelineRunStatus{PipelineRunStatusFields: v1.PipelineRunStatusFields{
					StartTime: &metav1.Time{Time: tc.starttime},
				}},
			}

			if pr.HasTimedOutForALongTime(t.Context(), testClock) != tc.expected {
				t.Errorf("Expected HasTimedOut to be %t when using pipeline.timeouts.pipeline", tc.expected)
			}
		})
	}
}

func TestPipelineRunHasTimedOut(t *testing.T) {
	tcs := []struct {
		name      string
		timeout   time.Duration
		starttime time.Time
		expected  bool
	}{{
		name:      "timedout",
		timeout:   1 * time.Second,
		starttime: now.AddDate(0, 0, -1),
		expected:  true,
	}, {
		name:      "nottimedout",
		timeout:   25 * time.Hour,
		starttime: now.AddDate(0, 0, -1),
		expected:  false,
	}, {
		name:      "notimeoutspecified",
		timeout:   0 * time.Second,
		starttime: now.AddDate(0, 0, -1),
		expected:  false,
	},
	}

	for _, tc := range tcs {
		t.Run("pipeline.timeouts.pipeline "+tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: v1.PipelineRunSpec{
					Timeouts: &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: tc.timeout}},
				},
				Status: v1.PipelineRunStatus{PipelineRunStatusFields: v1.PipelineRunStatusFields{
					StartTime: &metav1.Time{Time: tc.starttime},
				}},
			}

			if pr.HasTimedOut(t.Context(), testClock) != tc.expected {
				t.Errorf("Expected HasTimedOut to be %t when using pipeline.timeouts.pipeline", tc.expected)
			}
		})
		t.Run("pipeline.timeouts.tasks "+tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: v1.PipelineRunSpec{
					Timeouts: &v1.TimeoutFields{Tasks: &metav1.Duration{Duration: tc.timeout}},
				},
				Status: v1.PipelineRunStatus{PipelineRunStatusFields: v1.PipelineRunStatusFields{
					StartTime: &metav1.Time{Time: tc.starttime},
				}},
			}

			if pr.HaveTasksTimedOut(t.Context(), testClock) != tc.expected {
				t.Errorf("Expected HasTimedOut to be %t when using pipeline.timeouts.pipeline", tc.expected)
			}
		})
		t.Run("pipeline.timeouts.finally "+tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: v1.PipelineRunSpec{
					Timeouts: &v1.TimeoutFields{Finally: &metav1.Duration{Duration: tc.timeout}},
				},
				Status: v1.PipelineRunStatus{PipelineRunStatusFields: v1.PipelineRunStatusFields{
					StartTime:        &metav1.Time{Time: tc.starttime},
					FinallyStartTime: &metav1.Time{Time: tc.starttime},
				}},
			}

			if pr.HasFinallyTimedOut(t.Context(), testClock) != tc.expected {
				t.Errorf("Expected HasTimedOut to be %t when using pipeline.timeouts.pipeline", tc.expected)
			}
		})
	}
}

func TestPipelineRunTimeouts(t *testing.T) {
	tcs := []struct {
		name                   string
		timeouts               *v1.TimeoutFields
		expectedTasksTimeout   *metav1.Duration
		expectedFinallyTimeout *metav1.Duration
	}{{
		name: "no timeouts",
	}, {
		name:     "pipeline timeout set",
		timeouts: &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: time.Minute}},
	}, {
		name:                   "pipeline and tasks timeout set",
		timeouts:               &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: time.Hour}, Tasks: &metav1.Duration{Duration: 10 * time.Minute}},
		expectedTasksTimeout:   &metav1.Duration{Duration: 10 * time.Minute},
		expectedFinallyTimeout: &metav1.Duration{Duration: 50 * time.Minute},
	}, {
		name:                   "pipeline and finally timeout set",
		timeouts:               &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: time.Hour}, Finally: &metav1.Duration{Duration: 10 * time.Minute}},
		expectedTasksTimeout:   &metav1.Duration{Duration: 50 * time.Minute},
		expectedFinallyTimeout: &metav1.Duration{Duration: 10 * time.Minute},
	}, {
		name:                 "tasks timeout set",
		timeouts:             &v1.TimeoutFields{Tasks: &metav1.Duration{Duration: 10 * time.Minute}},
		expectedTasksTimeout: &metav1.Duration{Duration: 10 * time.Minute},
	}, {
		name:                   "finally timeout set",
		timeouts:               &v1.TimeoutFields{Finally: &metav1.Duration{Duration: 10 * time.Minute}},
		expectedFinallyTimeout: &metav1.Duration{Duration: 10 * time.Minute},
	}, {
		name:                 "no tasks timeout",
		timeouts:             &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: 0}, Tasks: &metav1.Duration{Duration: 0}},
		expectedTasksTimeout: &metav1.Duration{Duration: 0},
	}, {
		name:                   "no finally timeout",
		timeouts:               &v1.TimeoutFields{Pipeline: &metav1.Duration{Duration: 0}, Finally: &metav1.Duration{Duration: 0}},
		expectedFinallyTimeout: &metav1.Duration{Duration: 0},
	}}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "foo"},
				Spec: v1.PipelineRunSpec{
					Timeouts: tc.timeouts,
				},
			}

			tasksTimeout := pr.TasksTimeout()
			if ok := cmp.Equal(tc.expectedTasksTimeout, pr.TasksTimeout()); !ok {
				t.Errorf("Unexpected tasks timeout %v, expected %v", tasksTimeout, tc.expectedTasksTimeout)
			}
			finallyTimeout := pr.FinallyTimeout()
			if ok := cmp.Equal(tc.expectedFinallyTimeout, pr.FinallyTimeout()); !ok {
				t.Errorf("Unexpected finally timeout %v, expected %v", finallyTimeout, tc.expectedFinallyTimeout)
			}
		})
	}
}

func TestPipelineRunGetPodSpecSABackcompatibility(t *testing.T) {
	for _, tt := range []struct {
		name        string
		pr          *v1.PipelineRun
		expectedSAs map[string]string
	}{
		{
			name: "test backward compatibility",
			pr: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pr"},
				Spec: v1.PipelineRunSpec{
					PipelineRef: &v1.PipelineRef{Name: "prs"},
					TaskRunTemplate: v1.PipelineTaskRunTemplate{
						ServiceAccountName: "defaultSA",
					},
					TaskRunSpecs: []v1.PipelineTaskRunSpec{{
						PipelineTaskName:   "taskName",
						ServiceAccountName: "newTaskSA",
					}},
				},
			},
			expectedSAs: map[string]string{
				"unknown":  "defaultSA",
				"taskName": "newTaskSA",
			},
		}, {
			name: "mixed default SA backward compatibility",
			pr: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pr"},
				Spec: v1.PipelineRunSpec{
					PipelineRef: &v1.PipelineRef{Name: "prs"},
					TaskRunTemplate: v1.PipelineTaskRunTemplate{
						ServiceAccountName: "defaultSA",
					},
					TaskRunSpecs: []v1.PipelineTaskRunSpec{{
						PipelineTaskName:   "taskNameOne",
						ServiceAccountName: "TaskSAOne",
					}, {
						PipelineTaskName:   "taskNameTwo",
						ServiceAccountName: "newTaskTwo",
					}},
				},
			},
			expectedSAs: map[string]string{
				"unknown":     "defaultSA",
				"taskNameOne": "TaskSAOne",
				"taskNameTwo": "newTaskTwo",
			},
		}, {
			name: "mixed SA and TaskRunSpec",
			pr: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pr"},
				Spec: v1.PipelineRunSpec{
					PipelineRef: &v1.PipelineRef{Name: "prs"},
					TaskRunTemplate: v1.PipelineTaskRunTemplate{
						ServiceAccountName: "defaultSA",
					},
					TaskRunSpecs: []v1.PipelineTaskRunSpec{{
						PipelineTaskName: "taskNameOne",
					}, {
						PipelineTaskName:   "taskNameTwo",
						ServiceAccountName: "newTaskTwo",
					}},
				},
			},
			expectedSAs: map[string]string{
				"unknown":     "defaultSA",
				"taskNameOne": "defaultSA",
				"taskNameTwo": "newTaskTwo",
			},
		},
	} {
		for taskName, expected := range tt.expectedSAs {
			t.Run(tt.name, func(t *testing.T) {
				s := tt.pr.GetTaskRunSpec(taskName)
				if expected != s.ServiceAccountName {
					t.Errorf("wrong service account: got: %v, want: %v", s.ServiceAccountName, expected)
				}
			})
		}
	}
}

func TestPipelineRunGetPodSpec(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		pr                   *v1.PipelineRun
		expectedPodTemplates map[string][]string
	}{
		{
			name: "mix default and none default",
			pr: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pr"},
				Spec: v1.PipelineRunSpec{
					PipelineRef: &v1.PipelineRef{Name: "prs"},
					TaskRunTemplate: v1.PipelineTaskRunTemplate{
						ServiceAccountName: "defaultSA",
						PodTemplate:        &pod.Template{SchedulerName: "scheduleTest"},
					},
					TaskRunSpecs: []v1.PipelineTaskRunSpec{{
						PipelineTaskName:   "taskNameOne",
						ServiceAccountName: "TaskSAOne",
						PodTemplate:        &pod.Template{SchedulerName: "scheduleTestOne"},
					}, {
						PipelineTaskName:   "taskNameTwo",
						ServiceAccountName: "newTaskTwo",
						PodTemplate:        &pod.Template{SchedulerName: "scheduleTestTwo"},
					}},
				},
			},
			expectedPodTemplates: map[string][]string{
				"unknown":     {"scheduleTest", "defaultSA"},
				"taskNameOne": {"scheduleTestOne", "TaskSAOne"},
				"taskNameTwo": {"scheduleTestTwo", "newTaskTwo"},
			},
		},
	} {
		for taskName, values := range tt.expectedPodTemplates {
			t.Run(tt.name, func(t *testing.T) {
				s := tt.pr.GetTaskRunSpec(taskName)
				if values[0] != s.PodTemplate.SchedulerName {
					t.Errorf("wrong task podtemplate scheduler name: got: %v, want: %v", s.PodTemplate.SchedulerName, values[0])
				}
				if values[1] != s.ServiceAccountName {
					t.Errorf("wrong service account: got: %v, want: %v", s.ServiceAccountName, values[1])
				}
			})
		}
	}
}

func TestPipelineRun_GetTaskRunSpec(t *testing.T) {
	user := int64(1000)
	group := int64(2000)
	fsGroup := int64(3000)
	for _, tt := range []struct {
		name                 string
		pr                   *v1.PipelineRun
		expectedPodTemplates map[string]*pod.PodTemplate
	}{
		{
			name: "pipelineRun Spec podTemplate and taskRunSpec pipelineTask podTemplate",
			pr: &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{Name: "pr"},
				Spec: v1.PipelineRunSpec{
					TaskRunTemplate: v1.PipelineTaskRunTemplate{
						PodTemplate: &pod.Template{
							SecurityContext: &corev1.PodSecurityContext{
								RunAsUser:  &user,
								RunAsGroup: &group,
								FSGroup:    &fsGroup,
							},
						},
						ServiceAccountName: "defaultSA",
					},
					PipelineRef: &v1.PipelineRef{Name: "prs"},
					TaskRunSpecs: []v1.PipelineTaskRunSpec{{
						PipelineTaskName:   "task-1",
						ServiceAccountName: "task-1-service-account",
						PodTemplate: &pod.Template{
							NodeSelector: map[string]string{
								"diskType": "ssd",
							},
						},
					}, {
						PipelineTaskName:   "task-2",
						ServiceAccountName: "task-2-service-account",
						PodTemplate: &pod.Template{
							SchedulerName: "task-2-schedule",
						},
					}},
				},
			},
			expectedPodTemplates: map[string]*pod.PodTemplate{
				"task-1": {
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  &user,
						RunAsGroup: &group,
						FSGroup:    &fsGroup,
					},
					NodeSelector: map[string]string{
						"diskType": "ssd",
					},
				},
				"task-2": {
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  &user,
						RunAsGroup: &group,
						FSGroup:    &fsGroup,
					},
					SchedulerName: "task-2-schedule",
				},
			},
		},
	} {
		for taskName := range tt.expectedPodTemplates {
			t.Run(tt.name, func(t *testing.T) {
				s := tt.pr.GetTaskRunSpec(taskName)
				if d := cmp.Diff(tt.expectedPodTemplates[taskName], s.PodTemplate); d != "" {
					t.Error(diff.PrintWantGot(d))
				}
			})
		}
	}
}

func TestPipelineRunMarkFailedCondition(t *testing.T) {
	failedRunReason := v1.PipelineRunReasonFailed
	messageFormat := "error bar occurred %s"

	makeMessages := func(hasUserError bool) []interface{} {
		errorMsg := "baz error message"
		original := errors.New("orignal error")

		messages := make([]interface{}, 0)
		if hasUserError {
			messages = append(messages, pipelineErrors.WrapUserError(original))
		} else {
			messages = append(messages, errorMsg)
		}

		return messages
	}

	tcs := []struct {
		name               string
		hasUserError       bool
		prStatus           v1.PipelineRunStatus
		expectedConditions duckv1.Conditions
	}{{
		name:         "mark pipelinerun status failed with user error",
		hasUserError: true,
		prStatus: v1.PipelineRunStatus{
			PipelineRunStatusFields: v1.PipelineRunStatusFields{
				StartTime: &metav1.Time{Time: now},
			},
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{},
			},
		},
		expectedConditions: duckv1.Conditions{
			apis.Condition{
				Type:    "Succeeded",
				Status:  "False",
				Reason:  "Failed",
				Message: "[User error] error bar occurred orignal error",
			},
		},
	}, {
		name:         "mark pipelinerun status failed non user error",
		hasUserError: false,
		prStatus: v1.PipelineRunStatus{
			PipelineRunStatusFields: v1.PipelineRunStatusFields{
				StartTime: &metav1.Time{Time: now},
			},
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{},
			},
		},
		expectedConditions: duckv1.Conditions{
			apis.Condition{
				Type:    "Succeeded",
				Status:  "False",
				Reason:  "Failed",
				Message: "error bar occurred baz error message",
			},
		},
	}}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			pr := &v1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Status: tc.prStatus,
			}
			pr.Status.MarkFailed(failedRunReason.String(), messageFormat, makeMessages(tc.hasUserError)...)
			updatedCondition := pr.Status.Status.Conditions

			if d := cmp.Diff(tc.expectedConditions, updatedCondition, cmpopts.IgnoreFields(apis.Condition{}, "LastTransitionTime")); d != "" {
				t.Error(diff.PrintWantGot(d))
			}
		})
	}
}
