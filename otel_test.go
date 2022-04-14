package cobrautil

import (
	"testing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

func toMap(res *resource.Resource) map[string]string {
	m := map[string]string{}
	for _, attr := range res.Attributes() {
		m[string(attr.Key)] = attr.Value.Emit()
	}
	return m
}

func Test_setResource(t *testing.T) {
	type args struct {
		serviceName string
	}
	tests := []struct {
		name    string
		envVars map[string]string
		args    args
		want    map[string]string
	}{
		{
			name:    "withoutEnvVars",
			envVars: map[string]string{},
			args:    args{serviceName: "testService"},
			want: map[string]string{
				string(semconv.ServiceNameKey):           "testService",
				string(semconv.DeploymentEnvironmentKey): "",
			},
		},
		{
			name: "withEnvVars",
			envVars: map[string]string{
				"OTEL_RESOURCE_ATTRIBUTES": "deployment.environment=test",
			},
			args: args{serviceName: "testservice"},
			want: map[string]string{
				string(semconv.ServiceNameKey):           "testservice",
				string(semconv.DeploymentEnvironmentKey): "test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			got, err := setResource(tt.args.serviceName)
			if err != nil {
				t.Errorf("setResource() error = %v", err)
				return
			}

			gotMap := toMap(got)
			for k, v := range tt.want {
				if gotMap[k] != v {
					t.Errorf("setResource() mismatch for attribute %s: want: %s got: %s", k, v, gotMap[k])
				}
			}
		})
	}
}
