package options

type Option interface {
	GetLocation() string
	GetLocationConfigName() string
	String() string
}
