// Code generated by genny. DO NOT EDIT.
// This file was automatically generated by genny.
// Any changes will be lost if this file is regenerated.
// see https://github.com/mauricelam/genny

package protoutils

import "github.com/stackrox/rox/generated/storage"

// *storage.Signature represents a generic type that we use in the function below.

// ContainsStorageSignatureInSlice returns whether the given proto object is contained in the given slice.
func ContainsStorageSignatureInSlice(proto *storage.Signature, slice []*storage.Signature) bool {
	for _, elem := range slice {
		if protoEqualWrapper(proto, elem) {
			return true
		}
	}
	return false
}
