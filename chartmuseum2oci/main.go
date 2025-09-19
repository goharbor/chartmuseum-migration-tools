package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/goharbor/go-client/pkg/harbor"
	assistClient "github.com/goharbor/go-client/pkg/sdk/assist/client"
	"github.com/goharbor/go-client/pkg/sdk/assist/client/chart_repository"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/project"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/semver"
	"github.com/schollz/progressbar/v3"
)

type HelmChart struct {
	Name    string
	Project string
	Version string
}

func (hc HelmChart) ChartFileName() string {
	return fmt.Sprintf("%s-%s.tgz", hc.Name, hc.Version)
}

type ProjectsToMigrateList []string

const (
	fileMode        = 0o600
	helmBinaryPath  = "helm"
	timeout         = 5 * time.Second
	defaultPageSize = 10
	// Define minimum required version
	minVersion = "v3.19.0"
)

var (
	harborClientV2       *client.HarborAPI       //nolint:gochecknoglobals
	harborClientV2Assist *assistClient.HarborAPI //nolint:gochecknoglobals

	harborURL         string                //nolint:gochecknoglobals
	harborUsername    string                //nolint:gochecknoglobals
	harborPassword    string                //nolint:gochecknoglobals
	harborHost        string                //nolint:gochecknoglobals
	destPath          string                //nolint:gochecknoglobals
	projectsToMigrate ProjectsToMigrateList //nolint:gochecknoglobals

	insecure    bool //nolint:gochecknoglobals
	plainHttp   bool //nolint:gochecknoglobals
	showVersion bool //nolint:gochecknoglobals

	// Version information set during build
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func init() { //nolint:gochecknoinits
	initFlags()
	initHarborClients()
	initHarborHost()
}

func initFlags() {
	flag.StringVar(&harborURL, "url", "", "Harbor registry url")
	flag.StringVar(&harborUsername, "username", "", "Harbor registry username")
	flag.StringVar(&harborPassword, "password", "", "Harbor registry password")
	flag.StringVar(&destPath, "destpath", "", "Destination subpath")
	flag.Var(&projectsToMigrate, "project", "Name of the project(s) to migrate")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS verification for helm operations")
	flag.BoolVar(&plainHttp, "plain-http", false, "Use plain HTTP for helm operations")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.Parse()

	if showVersion {
		fmt.Printf("chartmuseum2oci version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("build date: %s\n", buildDate)
		os.Exit(0)
	}

	if harborURL == "" {
		log.Fatal(errors.New("Missing required --url flag"))
	}

	if harborUsername == "" {
		log.Fatal(errors.New("Missing required --username flag"))
	}

	if harborPassword == "" {
		log.Fatal(errors.New("Missing required --password flag"))
	}
}

func initHarborClients() {
	config := &harbor.ClientSetConfig{
		URL:      harborURL,
		Insecure: false,
		Username: harborUsername,
		Password: harborPassword,
	}

	harborClientSet, err := harbor.NewClientSet(config)
	if err != nil {
		log.Fatal(err, errors.Wrap(err, "fail to create harbor client"))
	}

	harborClientV2 = harborClientSet.V2() // v2 client
	harborClientV2Assist = harborClientSet.Assist()

	// Check Harbor url and credentials are ok
	params := &project.ListProjectsParams{} //nolint:exhaustruct
	if _, err = harborClientV2.Project.ListProjects(context.Background(), params); err != nil {
		log.Fatal(errors.Wrap(err, "fail to contact Harbor registry, check your credentials"))
	}
}

func initHarborHost() {
	u, err := url.Parse(harborURL)
	if err != nil {
		log.Fatal(errors.Wrapf(err, "fail to parse Harbor URL"))
	}

	harborHost = u.Host
}

// checkHelmVersion checks if Helm version meets the minimum requirement (>= 3.19.0)
func checkHelmVersion() error {
	cmd := exec.Command(helmBinaryPath, "version", "--short") //nolint:gosec

	var stdOut, stdErr bytes.Buffer
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "fail to execute helm version command: %s", stdErr.String())
	}

	versionOutput := strings.TrimSpace(stdOut.String())

	// Extract version from output like "v3.19.0+gce43812"
	versionStr := extractVersionFromOutput(versionOutput)
	if versionStr == "" {
		return errors.Errorf("unable to extract version from Helm output: %s", versionOutput)
	}

	// Validate version format using semver library
	if !semver.IsValid("v" + versionStr) {
		return errors.Errorf("invalid Helm version format: %s", versionStr)
	}

	// Check if version meets minimum requirement
	if semver.Compare("v"+versionStr, minVersion) < 0 {
		return errors.Errorf("Helm version %s is too old, requires version >= %s", versionStr, minVersion)
	}

	log.Printf("Helm version check passed: %s", versionStr)
	return nil
}

// extractVersionFromOutput extracts version string from Helm version output
func extractVersionFromOutput(output string) string {
	// Handle different Helm version output formats:
	// - "v3.19.0+gce43812" (with git commit)
	// - "v3.19.0" (without git commit)
	// - "3.19.0" (without 'v' prefix)

	// Remove 'v' prefix if present
	version := strings.TrimPrefix(output, "v")

	// Find the first '+' or end of string to get clean version
	if plusIndex := strings.Index(version, "+"); plusIndex != -1 {
		version = version[:plusIndex]
	}

	// Validate that it looks like a semantic version
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return ""
	}

	return version
}

func main() {
	if err := checkHelmVersion(); err != nil {
		log.Fatal(errors.Wrapf(err, "fail to check Helm version"))
	}

	if err := helmLogin(); err != nil {
		log.Fatal(errors.Wrapf(err, "fail to login to Helm"))
	}

	helmChartsToMigrate, err := getHarborChartmuseumCharts()
	if err != nil {
		log.Fatal(errors.Wrapf(err, "fail to retrieve helm charts to migrate"))
	}

	log.Printf("%d Helm charts to migrate from Chartmuseum to OCI", len(helmChartsToMigrate))

	bar := progressbar.Default(int64(len(helmChartsToMigrate)))
	errorCount := 0

	for _, helmChart := range helmChartsToMigrate {
		_ = bar.Add(1)

		if err := migrateChartFromChartmuseumToOCI(helmChart); err != nil {
			errorCount++

			log.Println(errors.Wrapf(err, "fail to migrate helm chart"))
		}
	}

	log.Printf("%d Helm charts successfully migrated from Chartmuseum to OCI", len(helmChartsToMigrate)-errorCount)
}

// helmLogin performs helm registry login with optional extra arguments
func helmLogin() error {
	params := []string{"registry", "login", "--username", harborUsername, "--password", harborPassword, harborURL}
	// Add extra arguments if provided
	if insecure {
		params = append(params, "--insecure")
	}

	if plainHttp {
		params = append(params, "--plain-http")
	}

	cmd := exec.Command(helmBinaryPath, params...) //nolint:gosec

	var stdErr bytes.Buffer
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "fail to execute helm login' command: %s", stdErr.String())
	}

	return nil
}

func getHarborChartmuseumCharts() ([]HelmChart, error) {
	helmCharts := make([]HelmChart, 0)

	pageSize := int64(defaultPageSize)
	for page := int64(1); true; page++ {
		params := &project.ListProjectsParams{Page: &page, PageSize: &pageSize} //nolint:exhaustruct

		projects, err := harborClientV2.Project.ListProjects(context.Background(), params)
		if err != nil {
			log.Fatal(errors.Wrapf(err, "fail to list harbor projects of page %d", page))
		}

		for _, harborProject := range projects.Payload {
			if len(projectsToMigrate) > 0 && !projectsToMigrate.Includes(harborProject.Name) {
				continue
			}

			projectHelmCharts, err := getHarborProjectChartmuseumCharts(harborProject.Name)
			if err != nil {
				return nil, errors.Wrapf(err, "fail to migrate charts from project %s", harborProject.Name)
			}

			helmCharts = append(helmCharts, projectHelmCharts...)
		}

		if projects.XTotalCount <= page**params.PageSize {
			break
		}
	}

	return helmCharts, nil
}

func getHarborProjectChartmuseumCharts(projectName string) ([]HelmChart, error) {
	helmCharts := make([]HelmChart, 0)

	params := &chart_repository.GetChartrepoRepoChartsParams{Repo: projectName} //nolint:exhaustruct

	charts, err := harborClientV2Assist.ChartRepository.GetChartrepoRepoCharts(context.Background(), params)
	if err != nil {
		return nil, errors.Wrap(err, "fail to list harbor projects")
	}

	for _, chart := range charts.Payload {
		params := &chart_repository.GetChartrepoRepoChartsNameParams{ //nolint:exhaustruct
			Repo: projectName,
			Name: *chart.Name,
		}

		chartVersions, err := harborClientV2Assist.ChartRepository.GetChartrepoRepoChartsName(context.Background(), params)
		if err != nil {
			return nil, errors.Wrapf(err, "fail to get chart %s in project %s", *chart.Name, projectName)
		}

		for _, chartVersion := range chartVersions.Payload {
			helmChart := HelmChart{
				Project: projectName,
				Name:    *chart.Name,
				Version: *chartVersion.Version,
			}

			helmCharts = append(helmCharts, helmChart)
		}
	}

	return helmCharts, nil
}

func migrateChartFromChartmuseumToOCI(helmChart HelmChart) error {
	if err := pullChartFromChartmuseum(helmChart); err != nil {
		return errors.Wrapf(err, "fail to pull chart from chartmuseum")
	}

	if err := pushChartToOCI(helmChart); err != nil {
		return errors.Wrapf(err, "fail to push chart to OCI")
	}

	if err := removeChartFile(helmChart); err != nil {
		return errors.Wrapf(err, "fail to remove chart file")
	}

	return nil
}

func pullChartFromChartmuseum(helmChart HelmChart) error {
	chartFileName := helmChart.ChartFileName()
	url := fmt.Sprintf("%s/chartrepo/%s/charts/%s", harborURL, helmChart.Project, chartFileName)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrapf(err, "fail to pull chart from Chartmuseum: %s", url)
	}

	req.SetBasicAuth(harborUsername, harborPassword)

	httpClient := http.Client{Timeout: timeout} //nolint:exhaustruct

	res, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "fail to retrieve chart from chartmuseum: %s", url)
	}

	if res.StatusCode != http.StatusOK {
		err := fmt.Errorf("received status %d", res.StatusCode) //nolint:goerr113

		return errors.Wrapf(err, "fail to retrieve chart from chartmuseum: %s", url)
	}

	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return errors.Wrapf(err, "fail to read chart body: %s", url)
	}

	err = os.WriteFile(chartFileName, resBody, fileMode)

	return errors.Wrapf(err, "fail to write chart file to disk: %s", url)
}

func pushChartToOCI(helmChart HelmChart) error {
	repoURL := fmt.Sprintf("oci://%s/%s%s", harborHost, helmChart.Project, destPath)
	params := []string{"push", helmChart.ChartFileName(), repoURL}
	if insecure {
		params = append(params, "--insecure-skip-tls-verify")
	}

	if plainHttp {
		params = append(params, "--plain-http")
	}

	cmd := exec.Command(helmBinaryPath, params...) //nolint:gosec

	var stdErr bytes.Buffer
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "fail to execute helm push' command: %s for url: %s and file: %s", stdErr.String(), repoURL, helmChart.ChartFileName()) //nolint:lll
	}

	return nil
}

func removeChartFile(helmChart HelmChart) error {
	chartFileName := helmChart.ChartFileName()

	err := os.Remove(chartFileName)

	return errors.Wrapf(err, "fail to delete file %s", chartFileName)
}

func (l *ProjectsToMigrateList) Set(value string) error {
	*l = append(*l, value)

	return nil
}

func (l *ProjectsToMigrateList) String() string {
	return ""
}

func (l ProjectsToMigrateList) Includes(a string) bool {
	for _, b := range l {
		if b == a {
			return true
		}
	}

	return false
}
