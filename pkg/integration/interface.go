package integration

type IntegrationInterface interface {
	GetIntegrationToken() (IntegrationToken, error)
}

type IntegrationToken interface {
	GetType() string

	// TODO Builds an AWS S3 or TencentCloud endpoint object with Bucket and Endpoint properties
	// GetEndpoint()
}
