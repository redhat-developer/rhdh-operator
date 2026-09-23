package template

// Mock implementations for template tests (exported for use in other packages)

type MockBackstageCR struct {
	Name      string
	Namespace string
}

func (m *MockBackstageCR) GetName() string {
	return m.Name
}

func (m *MockBackstageCR) GetNamespace() string {
	return m.Namespace
}

type MockPlatform struct {
	Extension string
}

func (m *MockPlatform) GetExtension() string {
	return m.Extension
}

type MockExternalConfig struct {
	IngressDomain string
}

func (m *MockExternalConfig) GetIngressDomain() string {
	return m.IngressDomain
}
