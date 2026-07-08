package discord

// tuicord patch — see third_party/arikawa-select-valuelimits.patch in the
// consuming repository.
//
// Upstream tags ValueLimits `json:"-"` on the select-family components and
// only writes min_values/max_values in the custom MarshalJSON methods. There
// is no unmarshal counterpart, so the limits of every component received from
// the gateway or REST API are silently dropped. Clients then cannot tell a
// multi-select (max_values > 1) apart from a single select. The UnmarshalJSON
// implementations below restore the limits on decode; absent fields leave the
// zero value, matching upstream's "zero means default [1, 1]" convention.

import "encoding/json"

func unmarshalWithValueLimits(b []byte, dst any, limits *[2]int) error {
	if err := json.Unmarshal(b, dst); err != nil {
		return err
	}
	var lim struct {
		MinValues *int `json:"min_values"`
		MaxValues *int `json:"max_values"`
	}
	if err := json.Unmarshal(b, &lim); err != nil {
		return err
	}
	if lim.MinValues != nil {
		limits[0] = *lim.MinValues
	}
	if lim.MaxValues != nil {
		limits[1] = *lim.MaxValues
	}
	return nil
}

// UnmarshalJSON decodes the select and preserves min_values/max_values.
func (s *StringSelectComponent) UnmarshalJSON(b []byte) error {
	type raw StringSelectComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the select and preserves min_values/max_values.
func (s *UserSelectComponent) UnmarshalJSON(b []byte) error {
	type raw UserSelectComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the select and preserves min_values/max_values.
func (s *RoleSelectComponent) UnmarshalJSON(b []byte) error {
	type raw RoleSelectComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the select and preserves min_values/max_values.
func (s *MentionableSelectComponent) UnmarshalJSON(b []byte) error {
	type raw MentionableSelectComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the select and preserves min_values/max_values.
func (s *ChannelSelectComponent) UnmarshalJSON(b []byte) error {
	type raw ChannelSelectComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the upload and preserves min_values/max_values.
func (s *FileUploadComponent) UnmarshalJSON(b []byte) error {
	type raw FileUploadComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}

// UnmarshalJSON decodes the group and preserves min_values/max_values.
func (s *CheckboxGroupComponent) UnmarshalJSON(b []byte) error {
	type raw CheckboxGroupComponent
	return unmarshalWithValueLimits(b, (*raw)(s), &s.ValueLimits)
}
