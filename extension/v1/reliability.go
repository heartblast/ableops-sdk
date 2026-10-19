package extensionv1

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// AllowsMethod is an exact declaration check. GET also admits HEAD as required
// by net/http. Empty methods retain legacy Host runtime discovery.
func (r RouteDecl) AllowsMethod(method string) bool {
	if len(r.Methods) == 0 {
		return true
	}
	for _, allowed := range r.Methods {
		if allowed == method || allowed == http.MethodGet && method == http.MethodHead {
			return true
		}
	}
	return false
}
func (m Manifest) validateReliability() error {
	var errs []error
	for _, route := range m.Routes {
		seen := map[string]bool{}
		for _, method := range route.Methods {
			switch method {
			case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
			default:
				errs = append(errs, fmt.Errorf("routes[].methods contains unsupported method"))
			}
			if seen[method] {
				errs = append(errs, errors.New("routes[].methods contains duplicate method"))
			}
			seen[method] = true
		}
	}
	for _, field := range m.Config.Schema {
		if field.Type == ConfigTypeSecretRef && (field.Secret || field.Default != nil) {
			errs = append(errs, errors.New("secretRef fields cannot contain secrets or package defaults"))
		}
		if len(field.Options) > 0 && field.Type != ConfigTypeSelect {
			errs = append(errs, errors.New("config options require select type"))
		}
		if len(field.Options) > 128 {
			errs = append(errs, errors.New("too many config select options"))
		}
		seen := map[string]bool{}
		for _, option := range field.Options {
			if strings.TrimSpace(option.Value) == "" || len(option.Value) > 4096 || seen[option.Value] {
				errs = append(errs, errors.New("invalid or duplicate select option"))
			}
			seen[option.Value] = true
		}
		if field.Default != nil && len(field.Options) > 0 {
			value, ok := field.Default.(string)
			if !ok || !seen[value] {
				errs = append(errs, errors.New("select default is outside declared options"))
			}
		}
	}
	return errors.Join(errs...)
}
