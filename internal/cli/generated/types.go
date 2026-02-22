package generated

type ParamSpec struct {
	Name        string
	In          string
	Required    bool
	Type        string
	Description string
}

type OperationSpec struct {
	Name         string
	OperationID  string
	Method       string
	Path         string
	Summary      string
	Description  string
	BodyRequired bool
	SupportsJSON bool
	JSONFields   []string
	Params       []ParamSpec
}

type TagSpec struct {
	Name       string
	Operations []OperationSpec
}
