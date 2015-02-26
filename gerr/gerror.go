package gerr

import (
	"fmt"
	"path"
	"runtime"
)

func New(reason string, symptom ...string) error {
	return new(pkg(), reason, combineSymptoms(symptom))
}

func Newf(reason string, symptomsFormat string, symptom ...interface{}) error {
	return new(pkg(), reason, fmt.Sprintf(symptomsFormat, symptom...))
}

func NewFromError(reason string, err error, symptom ...string) error {
    combined := combineSymptoms(symptom)
    var s string
    if combined == "" {
        s = fmt.Sprintf("%v", err)
    } else {
        s = fmt.Sprintf("%v: %v", combined, err)
    }
	return new(pkg(), reason, s)
}

func combineSymptoms(symptom []string) string {
    var s string
    switch len(symptom) {
        case 0:
        s = ""
        case 1:
        s = symptom[0]
        default:
        s = fmt.Sprintf("%v", symptom)
    }
    return s
}

func new(pkg string, action string, symptoms string) error {
	if symptoms == "" {
		return fmt.Errorf("%v: %v", pkg, action)

	}
	return fmt.Errorf("%v: %v: %v", pkg, action, symptoms)
}

func pkg() string {
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return "Cannot determine package"
	}

	return path.Base(path.Dir(file))
}
