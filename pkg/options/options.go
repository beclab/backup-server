package options

type Option interface {
	GetRepoName() string
	GetLocation() string
	GetLocationConfigName() string
}
