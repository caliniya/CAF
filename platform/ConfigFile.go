package platform

var MainConfigFile ConfigFile

var DefaultFile string

type ConfigFile interface {
	LoadDefaultFile() error
}
