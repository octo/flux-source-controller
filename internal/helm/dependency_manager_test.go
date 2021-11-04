/*
Copyright 2020 The Flux authors

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

package helm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/repo"
)

var (
	// helmPackageFile contains the path to a Helm package in the v2 format
	// without any dependencies
	helmPackageFile      = "testdata/charts/helmchart-0.1.0.tgz"
	chartName            = "helmchart"
	chartVersion         = "0.1.0"
	chartLocalRepository = "file://../helmchart"
	remoteDepFixture     = helmchart.Dependency{
		Name:       chartName,
		Version:    chartVersion,
		Repository: "https://example.com/charts",
	}
	// helmPackageV1File contains the path to a Helm package in the v1 format,
	// including dependencies in a requirements.yaml file which should be
	// loaded
	helmPackageV1File = "testdata/charts/helmchartwithdeps-v1-0.3.0.tgz"
	chartNameV1       = "helmchartwithdeps-v1"
	chartVersionV1    = "0.3.0"
)

func TestDependencyManager_Build(t *testing.T) {
	tests := []struct {
		name                       string
		baseDir                    string
		path                       string
		repositories               map[string]*ChartRepository
		getChartRepositoryCallback GetChartRepositoryCallback
		want                       int
		wantChartFunc              func(g *WithT, c *helmchart.Chart)
		wantErr                    string
	}{
		//{
		//	// TODO(hidde): add various happy paths
		//},
		//{
		//	// TODO(hidde): test Chart.lock
		//},
		{
			name:    "build failure returns error",
			baseDir: "testdata/charts",
			path:    "helmchartwithdeps",
			wantErr: "failed to add remote dependency 'grafana': no chart repository for URL",
		},
		{
			name:    "no dependencies returns zero",
			baseDir: "testdata/charts",
			path:    "helmchart",
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			chart, err := loader.Load(filepath.Join(tt.baseDir, tt.path))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := NewDependencyManager(chart, tt.baseDir, tt.path).
				WithRepositories(tt.repositories).
				WithChartRepositoryCallback(tt.getChartRepositoryCallback).
				Build(context.TODO())

			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				g.Expect(got).To(BeZero())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
			if tt.wantChartFunc != nil {
				tt.wantChartFunc(g, chart)
			}
		})
	}
}

func TestDependencyManager_build(t *testing.T) {
	tests := []struct {
		name    string
		deps    map[string]*helmchart.Dependency
		wantErr string
	}{
		{
			name: "error remote dependency",
			deps: map[string]*helmchart.Dependency{
				"example": {Repository: "https://example.com"},
			},
			wantErr: "failed to add remote dependency",
		},
		{
			name: "error local dependency",
			deps: map[string]*helmchart.Dependency{
				"example": {Repository: "file:///invalid"},
			},
			wantErr: "failed to add remote dependency",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dm := &DependencyManager{
				baseDir: "testdata/charts",
			}
			err := dm.build(context.TODO(), tt.deps)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestDependencyManager_addLocalDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      *helmchart.Dependency
		wantErr  string
		wantFunc func(g *WithT, c *helmchart.Chart)
	}{
		{
			name: "local dependency",
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    chartVersion,
				Repository: "file://../helmchart",
			},
			wantFunc: func(g *WithT, c *helmchart.Chart) {
				g.Expect(c.Dependencies()).To(HaveLen(1))
			},
		},
		{
			name: "version not matching constraint",
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    "0.2.0",
				Repository: "file://../helmchart",
			},
			wantErr: "can't get a valid version for constraint '0.2.0'",
		},
		{
			name: "invalid local reference",
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    chartVersion,
				Repository: "file://../../../absolutely/invalid",
			},
			wantErr: "no chart found at 'absolutely/invalid'",
		},
		{
			name: "invalid chart archive",
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    chartVersion,
				Repository: "file://../empty.tgz",
			},
			wantErr: "failed to load chart from 'empty.tgz'",
		},
		{
			name: "invalid constraint",
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    "invalid",
				Repository: "file://../helmchart",
			},
			wantErr: "invalid version/constraint format 'invalid'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dm := &DependencyManager{
				chart:   &helmchart.Chart{},
				baseDir: "testdata/charts/",
				path:    "helmchartwithdeps",
			}

			err := dm.addLocalDependency(tt.dep)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestDependencyManager_addRemoteDependency(t *testing.T) {
	g := NewWithT(t)

	chartB, err := os.ReadFile("testdata/charts/helmchart-0.1.0.tgz")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(chartB).ToNot(BeEmpty())

	tests := []struct {
		name         string
		repositories map[string]*ChartRepository
		dep          *helmchart.Dependency
		wantFunc     func(g *WithT, c *helmchart.Chart)
		wantErr      string
	}{
		{
			name: "adds remote dependency",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					Client: &mockGetter{
						response: chartB,
					},
					Index: &repo.IndexFile{
						Entries: map[string]repo.ChartVersions{
							chartName: {
								&repo.ChartVersion{
									Metadata: &helmchart.Metadata{
										Name:    chartName,
										Version: chartVersion,
									},
									URLs: []string{"https://example.com/foo.tgz"},
								},
							},
						},
					},
					RWMutex: &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Name:       chartName,
				Repository: "https://example.com",
			},
			wantFunc: func(g *WithT, c *helmchart.Chart) {
				g.Expect(c.Dependencies()).To(HaveLen(1))
			},
		},
		{
			name:         "resolve repository error",
			repositories: map[string]*ChartRepository{},
			dep: &helmchart.Dependency{
				Repository: "https://example.com",
			},
			wantErr: "no chart repository for URL",
		},
		{
			name: "strategic load error",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					CachePath: "/invalid/cache/path/foo",
					RWMutex:   &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Repository: "https://example.com",
			},
			wantErr: "failed to strategically load index",
		},
		{
			name: "repository get error",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					Index:   &repo.IndexFile{},
					RWMutex: &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Repository: "https://example.com",
			},
			wantErr: "no chart name found",
		},
		{
			name: "repository version constraint error",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					Index: &repo.IndexFile{
						Entries: map[string]repo.ChartVersions{
							chartName: {
								&repo.ChartVersion{
									Metadata: &helmchart.Metadata{
										Name:    chartName,
										Version: "0.1.0",
									},
								},
							},
						},
					},
					RWMutex: &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    "0.2.0",
				Repository: "https://example.com",
			},
			wantErr: fmt.Sprintf("no '%s' chart with version matching '0.2.0' found", chartName),
		},
		{
			name: "repository chart download error",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					Index: &repo.IndexFile{
						Entries: map[string]repo.ChartVersions{
							chartName: {
								&repo.ChartVersion{
									Metadata: &helmchart.Metadata{
										Name:    chartName,
										Version: chartVersion,
									},
								},
							},
						},
					},
					RWMutex: &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    chartVersion,
				Repository: "https://example.com",
			},
			wantErr: "chart download of version '0.1.0' failed",
		},
		{
			name: "chart load error",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {
					Client: &mockGetter{},
					Index: &repo.IndexFile{
						Entries: map[string]repo.ChartVersions{
							chartName: {
								&repo.ChartVersion{
									Metadata: &helmchart.Metadata{
										Name:    chartName,
										Version: chartVersion,
									},
									URLs: []string{"https://example.com/foo.tgz"},
								},
							},
						},
					},
					RWMutex: &sync.RWMutex{},
				},
			},
			dep: &helmchart.Dependency{
				Name:       chartName,
				Version:    chartVersion,
				Repository: "https://example.com",
			},
			wantErr: "failed to load downloaded archive of version '0.1.0'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dm := &DependencyManager{
				chart:        &helmchart.Chart{},
				repositories: tt.repositories,
			}
			err := dm.addRemoteDependency(tt.dep)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tt.wantFunc != nil {
				tt.wantFunc(g, dm.chart)
			}
		})
	}
}

func TestDependencyManager_resolveRepository(t *testing.T) {
	tests := []struct {
		name                       string
		repositories               map[string]*ChartRepository
		getChartRepositoryCallback GetChartRepositoryCallback
		url                        string
		want                       *ChartRepository
		wantRepositories           map[string]*ChartRepository
		wantErr                    string
	}{
		{
			name: "resolves from repositories index",
			url:  "https://example.com",
			repositories: map[string]*ChartRepository{
				"https://example.com/": {URL: "https://example.com"},
			},
			want: &ChartRepository{URL: "https://example.com"},
		},
		{
			name: "resolves from callback",
			url:  "https://example.com",
			getChartRepositoryCallback: func(url string) (*ChartRepository, error) {
				return &ChartRepository{URL: "https://example.com"}, nil
			},
			want: &ChartRepository{URL: "https://example.com"},
			wantRepositories: map[string]*ChartRepository{
				"https://example.com/": {URL: "https://example.com"},
			},
		},
		{
			name: "error from callback",
			url:  "https://example.com",
			getChartRepositoryCallback: func(url string) (*ChartRepository, error) {
				return nil, errors.New("a very unique error")
			},
			wantErr:          "a very unique error",
			wantRepositories: map[string]*ChartRepository{},
		},
		{
			name:    "error on not found",
			url:     "https://example.com",
			wantErr: "no chart repository for URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dm := &DependencyManager{
				repositories:               tt.repositories,
				getChartRepositoryCallback: tt.getChartRepositoryCallback,
			}

			got, err := dm.resolveRepository(tt.url)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
			if tt.wantRepositories != nil {
				g.Expect(dm.repositories).To(Equal(tt.wantRepositories))
			}
		})
	}
}

func TestDependencyManager_secureLocalChartPath(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		path    string
		dep     *helmchart.Dependency
		want    string
		wantErr string
	}{
		{
			name:    "secure local file path",
			baseDir: "/tmp/workdir",
			path:    "/chart",
			dep: &helmchart.Dependency{
				Repository: "../dep",
			},
			want: "/tmp/workdir/dep",
		},
		{
			name:    "insecure local file path",
			baseDir: "/tmp/workdir",
			path:    "/",
			dep: &helmchart.Dependency{
				Repository: "/../../dep",
			},
			want: "/tmp/workdir/dep",
		},
		{
			name: "URL parse error",
			dep: &helmchart.Dependency{
				Repository: ": //example.com",
			},
			wantErr: "missing protocol scheme",
		},
		{
			name: "error on URL scheme other than file",
			dep: &helmchart.Dependency{
				Repository: "https://example.com",
			},
			wantErr: "not a local chart reference",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dm := &DependencyManager{
				baseDir: tt.baseDir,
				path:    tt.path,
			}
			got, err := dm.secureLocalChartPath(tt.dep)
			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).ToNot(BeEmpty())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_collectMissing(t *testing.T) {
	tests := []struct {
		name    string
		current []*helmchart.Chart
		reqs    []*helmchart.Dependency
		want    map[string]*helmchart.Dependency
	}{
		{
			name:    "one missing",
			current: []*helmchart.Chart{},
			reqs: []*helmchart.Dependency{
				{Name: chartName},
			},
			want: map[string]*helmchart.Dependency{
				chartName: {Name: chartName},
			},
		},
		{
			name: "alias missing",
			current: []*helmchart.Chart{
				{
					Metadata: &helmchart.Metadata{
						Name: chartName,
					},
				},
			},
			reqs: []*helmchart.Dependency{
				{Name: chartName},
				{Name: chartName, Alias: chartName + "-alias"},
			},
			want: map[string]*helmchart.Dependency{
				chartName + "-alias": {Name: chartName, Alias: chartName + "-alias"},
			},
		},
		{
			name: "all current",
			current: []*helmchart.Chart{
				{
					Metadata: &helmchart.Metadata{
						Name: chartName,
					},
				},
			},
			reqs: []*helmchart.Dependency{
				{Name: chartName},
			},
			want: nil,
		},
		{
			name:    "nil",
			current: nil,
			reqs:    nil,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				g.Expect(collectMissing(tt.current, tt.reqs)).To(Equal(tt.want))
			})
		})
	}
}

func Test_isLocalDep(t *testing.T) {
	tests := []struct {
		name string
		dep  *helmchart.Dependency
		want bool
	}{
		{
			name: "file protocol",
			dep:  &helmchart.Dependency{Repository: "file:///some/path"},
			want: true,
		},
		{
			name: "empty",
			dep:  &helmchart.Dependency{Repository: ""},
			want: true,
		},
		{
			name: "https url",
			dep:  &helmchart.Dependency{Repository: "https://example.com"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isLocalDep(tt.dep)).To(Equal(tt.want))
		})
	}
}
