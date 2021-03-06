package init

import (
	"path/filepath"

	"github.com/haozzzzzzzz/go-tool/api/com/project"
	project2 "github.com/haozzzzzzzz/go-tool/api/com/source/project"
	"github.com/haozzzzzzzz/go-tool/api/com/source/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/go-playground/validator.v9"
)

// 初始化服务框架
func CommandApiInit() (cmd *cobra.Command) {
	var config project.ProjectConfigFormat

	cmd = &cobra.Command{
		Use:   "init",
		Short: "api project initialization",
		Run: func(cmd *cobra.Command, args []string) {
			err := validator.New().Struct(&config)
			if nil != err {
				logrus.Errorf("validate init config failed. %s.", err)
				return
			}

			absProjectDir, err := filepath.Abs(config.ProjectDir)
			if nil != err {
				logrus.Errorf("get absolute file path %q failed. %s.", absProjectDir, err)
				return
			}

			config.ProjectDir = absProjectDir

			proj := &project.Project{
				Config: &config,
			}

			err = proj.Init()
			if nil != err {
				logrus.Errorf("save project config failed. %s.", err)
				return
			}

			// init project service files
			apiProjectSource := project2.NewProjectSource(proj)
			err = apiProjectSource.Generate()
			if nil != err {
				logrus.Errorf("generate api project source failed. %s.", err)
				return
			}

		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&config.Name, "name", "n", "", "api project name")
	flags.StringVarP(&config.ProjectDir, "path", "p", "./", "api project directory path")

	return
}

// 初始化API服务
func CommandApiAddService() *cobra.Command {
	var config project.ServiceConfigFormat
	var params service.GenerateParams
	var cmd = &cobra.Command{
		Use:   "add_service",
		Short: "add api service",
		Run: func(cmd *cobra.Command, args []string) {
			err := validator.New().Struct(&config)
			if nil != err {
				logrus.Errorf("validate add service config failed. %s.", err)
				return
			}

			absServiceDir, err := filepath.Abs(config.ServiceDir)
			if nil != err {
				logrus.Errorf("get absolute file path %q failed. %s.", absServiceDir, err)
				return
			}
			config.ServiceDir = absServiceDir

			srv := &project.Service{
				Config: &config,
			}
			err = srv.Init()
			if nil != err {
				logrus.Errorf("save service config failed. %s.", err)
				return
			}

			// init api service files
			apiServiceSource := service.NewServiceSource(srv)
			err = apiServiceSource.Generate(&params)
			if nil != err {
				logrus.Errorf("generate api service source failed. %s.", err)
				return
			}

		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&config.Name, "name", "n", "", "api service name")
	flags.StringVarP(&config.ServiceDir, "path", "p", "./", "api service directory path")
	flags.StringVarP(&config.Description, "description", "d", "api service", "api service description")
	flags.StringVarP(&config.Type, "type", "t", "", "app、manage、rpc")
	flags.StringVarP(&params.Host, "host", "H", "", "api serve host")
	flags.StringVarP(&params.Port, "port", "P", "18100", "api serve port")

	return cmd
}
