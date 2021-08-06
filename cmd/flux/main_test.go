package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/fluxcd/flux2/internal/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/mattn/go-shellwords"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMain(m *testing.M) {
	// Ensure tests print consistent timestamps regardless of timezone
	os.Setenv("TZ", "UTC")
	os.Exit(m.Run())
}

func readYamlObjects(objectFile string) ([]client.Object, error) {
	obj, err := os.ReadFile(objectFile)
	if err != nil {
		return nil, err
	}
	objects := []client.Object{}
	reader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(obj)))
	for {
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
		}
		unstructuredObj := &unstructured.Unstructured{}
		decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(doc), len(doc))
		err = decoder.Decode(unstructuredObj)
		if err != nil {
			return nil, err
		}
		objects = append(objects, unstructuredObj)
	}
	return objects, nil
}

// A KubeManager that can create objects that are subject to a test.
type fakeKubeManager struct {
	fakeClient    client.WithWatch
	fakeClientset kubernetes.Interface
}

func (m *fakeKubeManager) NewClient(kubeconfig string, kubecontext string) (client.WithWatch, error) {
	return m.fakeClient, nil
}

func (m *fakeKubeManager) NewClientset(kubeconfig string, kubecontext string) (kubernetes.Interface, error) {
	return m.fakeClientset, nil
}

func (m *fakeKubeManager) CreateObjects(clientObjects []client.Object) error {
	for _, obj := range clientObjects {
		err := m.fakeClient.Create(context.Background(), obj)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewFakeKubeManager() *fakeKubeManager {
	c := fakeclient.NewClientBuilder().WithScheme(utils.NewScheme()).Build()
	cs := fakeclientset.NewSimpleClientset()
	return &fakeKubeManager{
		fakeClient:    c,
		fakeClientset: cs,
	}
}

// Run the command and return the captured output.
func executeCommand(cmd string) (string, error) {
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)

	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	_, err = rootCmd.ExecuteC()
	result := buf.String()

	return result, err
}

// Structure used for each test to load objects into kubernetes, run
// commands and assert on the expected output.
type cmdTestCase struct {
	// The command line arguments to test.
	args string
	// When true, the test expects the command to fail.
	wantError bool
	// String literal that contains the expected test output.
	goldenValue string
	// Filename that contains the expected test output.
	goldenFile string
	// Filename that contains yaml objects to load into Kubernetes
	objectFile string
}

func (cmd *cmdTestCase) runTestCmd(t *testing.T) {
	km := NewFakeKubeManager()
	rootCtx.kubeManager = km

	if cmd.objectFile != "" {
		clientObjects, err := readYamlObjects(cmd.objectFile)
		if err != nil {
			t.Fatalf("Error loading yaml: '%v'", err)
		}
		err = km.CreateObjects(clientObjects)
		if err != nil {
			t.Fatalf("Error creating test objects: '%v'", err)
		}
	}

	actual, err := executeCommand(cmd.args)
	if (err != nil) != cmd.wantError {
		t.Fatalf("Expected error='%v', Got: %v", cmd.wantError, err)
	}
	if err != nil {
		actual = err.Error()
	}

	var expected string
	if cmd.goldenValue != "" {
		expected = cmd.goldenValue
	}
	if cmd.goldenFile != "" {
		expectedOutput, err := os.ReadFile(cmd.goldenFile)
		if err != nil {
			t.Fatalf("Error reading golden file: '%s'", err)
		}
		expected = string(expectedOutput)
	}

	diff := cmp.Diff(expected, actual)
	if diff != "" {
		t.Errorf("Mismatch from '%s' (-want +got):\n%s", cmd.goldenFile, diff)
	}
}

func TestVersion(t *testing.T) {
	cmd := cmdTestCase{
		args:        "--version",
		goldenValue: "flux version 0.0.0-dev.0\n",
	}
	cmd.runTestCmd(t)
}
