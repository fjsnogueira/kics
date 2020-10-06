package test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/checkmarxDev/ice/pkg/engine"
	"github.com/checkmarxDev/ice/pkg/engine/mock"
	"github.com/checkmarxDev/ice/pkg/engine/query"
	"github.com/checkmarxDev/ice/pkg/model"
	"github.com/checkmarxDev/ice/pkg/parser"
	"github.com/golang/mock/gomock"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

const (
	fileID = 12345
	scanID = "testScan"

	queryExtension = ".rego"
	queriesDir     = "./../assets/queries"
	terraformDir   = "./test-data"

	expectedResultExtension = ".json"
	expectedResultsDir      = "./queries"
)

func TestQueries(t *testing.T) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: ioutil.Discard})

	queries := loadQueries(t, queriesDir)
	for _, q := range queries {
		name := strings.TrimSuffix(path.Base(q), path.Ext(q))
		t.Run(name, func(t *testing.T) {
			expectedResultFile := strings.ReplaceAll(strings.ToLower(path.Join(expectedResultsDir, q)), queryExtension, expectedResultExtension)
			content, err := ioutil.ReadFile(expectedResultFile)
			require.NoError(t, err, "can't read expected result file %s", expectedResultFile)

			var expectedVulnerabilities []model.Vulnerability
			err = json.Unmarshal(content, &expectedVulnerabilities)
			require.NoError(t, err, "can't unmarshal expected result file %s", expectedResultFile)

			testQuery(t, q, terraformFile(q), expectedVulnerabilities)
		})
		t.Run(name+"_success", func(t *testing.T) {
			testQuery(t, q, terraformSuccessFile(q), []model.Vulnerability{})
		})
	}
}

func loadQueries(t *testing.T, dir string) []string {
	infos := make([]string, 0)
	err := filepath.Walk(
		dir,
		func(p string, f os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if f.IsDir() || !strings.HasSuffix(f.Name(), queryExtension) {
				return nil
			}

			infos = append(infos, path.Join(filepath.Base(path.Dir(p)), f.Name()))
			return nil
		})

	require.NoError(t, err, "can't load queries")
	return infos
}

func testQuery(t *testing.T, queryPath, tfFilePath string, expectedVulnerabilities []model.Vulnerability) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.TODO()
	storage := mock.NewMockFilesStorage(ctrl)
	storage.EXPECT().GetFiles(gomock.Eq(ctx), gomock.Eq(scanID), gomock.Any()).
		DoAndReturn(func(ctx context.Context, scanID, filter string) (model.FileMetadatas, error) {
			f, err := readTestTerraformFile(scanID, fileID, tfFilePath)
			require.NoError(t, err)

			return model.FileMetadatas{f}, nil
		})

	storage.EXPECT().SaveVulnerabilities(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, results []model.Vulnerability) error {
			requireEqualVulnerabilities(t, expectedVulnerabilities, results)

			return nil
		})

	queriesSource := mock.NewMockQueriesSource(ctrl)
	queriesSource.EXPECT().GetQueries().
		DoAndReturn(func() ([]model.QueryMetadata, error) {
			q, err := query.ReadQuery(queriesDir, queryPath)
			require.NoError(t, err)

			return []model.QueryMetadata{q}, nil
		})

	inspector, err := engine.NewInspector(ctx, queriesSource, storage, engine.DefaultVulnerabilityBuilder)
	require.Nil(t, err)
	require.NotNil(t, inspector)

	err = inspector.Inspect(ctx, scanID)
	require.Nil(t, err)
}

func terraformFile(queryPath string) string {
	return strings.TrimSuffix(queryPath, filepath.Ext(queryPath)) + ".tf"
}

func terraformSuccessFile(queryPath string) string {
	return strings.TrimSuffix(queryPath, filepath.Ext(queryPath)) + "_success.tf"
}

func readTestTerraformFile(scanID string, fileID int, fileName string) (model.FileMetadata, error) {
	filePath := path.Join(terraformDir, fileName)
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return model.FileMetadata{}, err
	}

	jsonContent, err := parser.NewDefault().Parse(filePath, content)
	if err != nil {
		return model.FileMetadata{}, err
	}

	return model.FileMetadata{
		ID:           fileID,
		ScanID:       scanID,
		JSONData:     jsonContent,
		OriginalData: string(content),
		Kind:         model.KindTerraform,
		FileName:     filePath,
		JSONHash:     0,
	}, nil
}

func requireEqualVulnerabilities(t *testing.T, expected, actual []model.Vulnerability) {
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Line < expected[j].Line
	})
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].Line < actual[j].Line
	})

	require.Len(t, actual, len(expected), "Count of actual issues and expected vulnerabilities doesn't match")

	for i := range expected {
		if i > len(actual)-1 {
			t.Fatalf("Not enough results detected, expected %d, found %d", len(expected), len(actual))
		}

		expectedItem := expected[i]
		actualItem := actual[i]
		require.Equal(t, fileID, actualItem.FileID)
		require.Equal(t, scanID, actualItem.ScanID)
		require.Equal(t, expectedItem.Line, actualItem.Line, "Not corrected detected line")
		require.Equal(t, expectedItem.Severity, actualItem.Severity, "Invalid severity")
		require.Equal(t, expectedItem.QueryName, actualItem.QueryName, "Invalid query name")
		if expectedItem.Value != nil {
			require.NotNil(t, actualItem.Value)
			require.Equal(t, *expectedItem.Value, *actualItem.Value)
		}
	}
}