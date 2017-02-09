package model

import (
	"github.com/jinzhu/gorm"
	"github.com/rancher/go-rancher/client"
)

// TODO: might need a Base field for filtering
// TODO: might need a FolderName field for filtering
type Version struct {
	EnvironmentId string `json:"environmentId"`
	TemplateId    uint   `sql:"type:integer REFERENCES catalog_template(id) ON DELETE CASCADE"`

	Revision              int    `json:"revision"`
	Version               string `json:"version"`
	MinimumRancherVersion string `json:"minimumRancherVersion" yaml:"minimum_rancher_version"`
	MaximumRancherVersion string `json:"maximumRancherVersion" yaml:"maximum_rancher_version"`
	UpgradeFrom           string `json:"upgradeFrom" yaml:"upgrade_from"`

	// TODO move to model
	Files     []File
	Questions []Question
	Readme    string `json:"readme"`
}

type VersionModel struct {
	Base
	Version
}

type TemplateVersionResource struct {
	client.Resource
	Version

	Bindings            map[string]Bindings `json:"bindings"`
	Files               map[string]string   `json:"files"`
	Questions           []Question          `json:"questions"`
	UpgradeVersionLinks map[string]string   `json:"upgradeVersionLinks"`
}

// TODO: needs a base filter (make sure to use a map)
func LookupVersionModel(db *gorm.DB, environmentId, catalog, template string, revision int) *VersionModel {
	var versionModel VersionModel
	db.Raw(`
SELECT catalog_version.*
FROM catalog_version, catalog_template, catalog
WHERE (catalog_version.environment_id = ? OR catalog_version.environment_id = ?)
AND catalog_version.template_id = catalog_template.id
AND catalog_template.catalog_id = catalog.id
AND catalog.name = ?
AND catalog_template.folder_name = ?
AND catalog_version.revision = ?
`, environmentId, "global", catalog, template, revision).Scan(&versionModel)
	return &versionModel
}

func LookupVersions(db *gorm.DB, environmentId, catalog, template string) []Version {
	var versionModels []VersionModel
	db.Raw(`
SELECT catalog_version.*
FROM catalog_version, catalog_template, catalog
WHERE (catalog_version.environment_id = ? OR catalog_version.environment_id = ?)
AND catalog_version.template_id = catalog_template.id
AND catalog_template.catalog_id = catalog.id
AND catalog.name = ?
AND catalog_template.folder_name = ?
`, environmentId, "global", catalog, template).Scan(&versionModels)

	var versions []Version
	for _, versionModel := range versionModels {
		versions = append(versions, versionModel.Version)
	}
	return versions
}
