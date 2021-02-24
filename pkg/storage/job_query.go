package storage

import (
	"fmt"
	"reflect"

	"github.com/facebookincubator/contest/pkg/job"
)

type JobQueryField interface {
	queryFieldPointer(query *JobQuery) interface{}
}

type JobQueryFields []JobQueryField

type JobQuery struct {
	States []job.State
	Tags   []string
}

type jobQueryFieldStates []job.State
type jobQueryFieldTags []string

func QueryJobStates(states ...job.State) JobQueryField { return jobQueryFieldStates(states) }
func (value jobQueryFieldStates) queryFieldPointer(query *JobQuery) interface{} {
	return &query.States
}

func QueryJobTags(tags ...string) JobQueryField { return jobQueryFieldTags(tags) }
func (value jobQueryFieldTags) queryFieldPointer(query *JobQuery) interface{} {
	return &query.Tags
}

func BuildJobQuery(queryFields ...JobQueryField) (*JobQuery, error) {
	return JobQueryFields(queryFields).BuildQuery()
}

func (queryFields JobQueryFields) BuildQuery() (*JobQuery, error) {
	query := &JobQuery{}
	for idx, queryField := range queryFields {
		if err := ApplyJobQueryField(queryField.queryFieldPointer(query), queryField); err != nil {
			return nil, fmt.Errorf("unable to apply field %d:%T(%v): %w", idx, queryField, queryField, err)
		}
	}
	return query, nil
}

type ErrJobQueryFieldIsAlreadySet struct {
	FieldValue interface{}
	QueryField JobQueryField
}

func (err ErrJobQueryFieldIsAlreadySet) Error() string {
	return fmt.Sprintf("field %T is set multiple times: cur_value:%v new_value:%v",
		err.QueryField, err.FieldValue, err.QueryField)
}

// ErrQueryFieldHasZeroValue is returned when a QueryFields failed validation
// due to a QueryField with a zero value (this is unexpected and forbidden).
type ErrJobQueryFieldHasZeroValue struct {
	QueryField JobQueryField
}

func (err ErrJobQueryFieldHasZeroValue) Error() string {
	return fmt.Sprintf("field %T has a zero value", err.QueryField)
}

func ApplyJobQueryField(fieldPtr interface{}, queryField JobQueryField) error {
	if reflect.ValueOf(queryField).IsZero() {
		return ErrJobQueryFieldHasZeroValue{QueryField: queryField}
	}
	field := reflect.ValueOf(fieldPtr).Elem()
	if !reflect.ValueOf(field.Interface()).IsZero() {
		return ErrJobQueryFieldIsAlreadySet{FieldValue: field.Interface(), QueryField: queryField}
	}
	field.Set(reflect.ValueOf(queryField).Convert(field.Type()))
	return nil
}
