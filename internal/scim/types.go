package scim

const (
	UserSchema           = "urn:ietf:params:scim:schemas:core:2.0:User"
	GroupSchema          = "urn:ietf:params:scim:schemas:core:2.0:Group"
	PatchOpSchema        = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	ListResponseSchema   = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	ErrorSchema          = "urn:ietf:params:scim:api:messages:2.0:Error"
	EnterpriseUserSchema = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
)

type User struct {
	Schemas    []string `json:"schemas"`
	ID         string   `json:"id,omitempty"`
	ExternalID string   `json:"externalId,omitempty"`
	UserName   string   `json:"userName,omitempty"`
	Name       *Name    `json:"name,omitempty"`
	Emails     []Email  `json:"emails,omitempty"`
	Active     *bool    `json:"active,omitempty"`
	Title      string   `json:"title,omitempty"`

	PhoneNumbers []PhoneNumber `json:"phoneNumbers,omitempty"`

	// Enterprise extension attributes are flattened for convenience
	// but serialized under the extension schema URI key.
	EnterpriseUser *EnterpriseUser `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User,omitempty"`
}

type Name struct {
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

type Email struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type PhoneNumber struct {
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

type EnterpriseUser struct {
	Department string `json:"department,omitempty"`
}

type Group struct {
	Schemas     []string      `json:"schemas"`
	ID          string        `json:"id,omitempty"`
	ExternalID  string        `json:"externalId,omitempty"`
	DisplayName string        `json:"displayName"`
	Members     []GroupMember `json:"members,omitempty"`
}

type GroupMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

type PatchOp struct {
	Schemas    []string    `json:"schemas"`
	Operations []Operation `json:"Operations"`
}

type Operation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	Resources    []User   `json:"Resources"`
}

type GroupListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	Resources    []Group  `json:"Resources"`
}

type ErrorResponse struct {
	Schemas []string `json:"schemas"`
	Detail  string   `json:"detail"`
	Status  string   `json:"status"`
}

type ResourceType struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

func NewPatchOp(ops ...Operation) *PatchOp {
	return &PatchOp{
		Schemas:    []string{PatchOpSchema},
		Operations: ops,
	}
}

func BoolPtr(b bool) *bool {
	return &b
}
