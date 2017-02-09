package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/rancher/catalog-service/manager"
	"github.com/rancher/catalog-service/model"
	"github.com/rancher/catalog-service/service"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	refreshInterval int
	port            int
	cacheRoot       string
	configFile      string
	validateOnly    bool
	sqlite          bool
	migrateDb       bool
)

var RootCmd = &cobra.Command{
	Use: "catalog-service",
	Run: run,
}

func init() {
	viper.SetEnvPrefix("catalog_service")
	viper.AutomaticEnv()

	RootCmd.PersistentFlags().Int("refresh-interval", 60, "")
	viper.BindPFlag("refresh_interval", RootCmd.PersistentFlags().Lookup("refresh-interval"))

	RootCmd.PersistentFlags().IntVarP(&port, "port", "p", 8088, "")
	RootCmd.PersistentFlags().StringVar(&cacheRoot, "cache", "./cache", "")
	RootCmd.PersistentFlags().StringVar(&configFile, "config", "./repo.json", "")
	RootCmd.PersistentFlags().BoolVar(&validateOnly, "validate", false, "")
	RootCmd.PersistentFlags().BoolVar(&sqlite, "sqlite", false, "")
	RootCmd.PersistentFlags().BoolVar(&migrateDb, "migrate-db", false, "")

	RootCmd.PersistentFlags().String("mysql-user", "", "")
	viper.BindPFlag("mysql_user", RootCmd.PersistentFlags().Lookup("mysql-user"))

	RootCmd.PersistentFlags().String("mysql-password", "", "")
	viper.BindPFlag("mysql_password", RootCmd.PersistentFlags().Lookup("mysql-password"))

	RootCmd.PersistentFlags().String("mysql-address", "", "")
	viper.BindPFlag("mysql_address", RootCmd.PersistentFlags().Lookup("mysql-address"))

	RootCmd.PersistentFlags().String("mysql-dbname", "", "")
	viper.BindPFlag("mysql_dbname", RootCmd.PersistentFlags().Lookup("mysql-dbname"))

	RootCmd.PersistentFlags().String("mysql-params", "", "")
	viper.BindPFlag("mysql_params", RootCmd.PersistentFlags().Lookup("mysql-params"))
}

func run(cmd *cobra.Command, args []string) {
	config, err := readConfig(configFile)
	if err != nil {
		log.Fatal(err)
	}

	var db *gorm.DB
	if sqlite {
		db, err = gorm.Open("sqlite3", "local.db")
		if err != nil {
			log.Fatal(err)
		}
		db.Exec("PRAGMA foreign_keys = ON")
		migrateDb = true
	} else {
		user := viper.GetString("mysql_user")
		password := viper.GetString("mysql_password")
		address := viper.GetString("mysql_address")
		dbname := viper.GetString("mysql_dbname")
		params := viper.GetString("mysql_params")

		db, err = gorm.Open("mysql", formatDSN(user, password, address, dbname, params))
		if err != nil {
			log.Fatal(err)
		}
	}
	defer db.Close()

	db.SingularTable(true)
	gorm.DefaultTableNameHandler = func(db *gorm.DB, defaultTableName string) string {
		defaultTableName = strings.TrimSuffix(defaultTableName, "_model")
		if defaultTableName == "catalog" {
			return defaultTableName
		}
		return "catalog_" + defaultTableName
	}

	if migrateDb {
		log.Info("Migrating DB")
		db.AutoMigrate(&model.CatalogModel{})
		db.AutoMigrate(&model.TemplateModel{})
		db.AutoMigrate(&model.VersionModel{})
		db.AutoMigrate(&model.FileModel{})
	}

	m := manager.NewManager(cacheRoot, config, db)
	go refresh(m, refreshInterval, validateOnly)
	if validateOnly {
		select {}
	}

	log.Infof("Starting Catalog Service on port %d", port)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), &service.MuxWrapper{
		IsReady: false,
		Router:  service.NewRouter(manager.NewManager(cacheRoot, config, db), db),
	}))
}

func formatDSN(user, password, address, dbname, params string) string {
	paramsMap := map[string]string{
		"parseTime": "true",
	}
	for _, param := range strings.Split(params, "&") {
		split := strings.SplitN(param, "=", 2)
		if len(split) > 1 {
			paramsMap[split[0]] = split[1]
		}
	}
	mysqlConfig := &mysql.Config{
		User:   user,
		Passwd: password,
		Addr:   address,
		DBName: dbname,
		Params: paramsMap,
	}
	return mysqlConfig.FormatDSN()
}

func readConfig(configFile string) (map[string]manager.CatalogConfig, error) {
	configContents, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	var config map[string]map[string]manager.CatalogConfig
	if err = json.Unmarshal(configContents, &config); err != nil {
		return nil, err
	}
	return config["catalogs"], nil
}

func refresh(m *manager.Manager, refreshInterval int, validateOnly bool) {
	if err := m.CreateConfigCatalogs(); err != nil {
		log.Fatalf("Failed to create catalogs from config file: %v", err)
	}
	if err := m.RefreshAll(); err != nil {
		log.Fatalf("Failed to do initial refresh of catalogs: %v", err)
	}
	if validateOnly {
		os.Exit(0)
	}
	for range time.Tick(time.Duration(refreshInterval) * time.Second) {
		// TODO: don't want to have refresh running twice at the same time
		go m.RefreshAll()
	}
}