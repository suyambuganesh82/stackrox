// Code generated by "stringer -type=Op"; DO NOT EDIT.

package metrics

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[Add-0]
	_ = x[AddMany-1]
	_ = x[Count-2]
	_ = x[Dedupe-3]
	_ = x[Exists-4]
	_ = x[Get-5]
	_ = x[GetAll-6]
	_ = x[GetMany-7]
	_ = x[GetGrouped-8]
	_ = x[List-9]
	_ = x[Prune-10]
	_ = x[Reset-11]
	_ = x[Rename-12]
	_ = x[Remove-13]
	_ = x[RemoveMany-14]
	_ = x[Search-15]
	_ = x[SearchAndGet-16]
	_ = x[Update-17]
	_ = x[UpdateMany-18]
	_ = x[Upsert-19]
	_ = x[UpsertAll-20]
}

const _Op_name = "AddAddManyCountDedupeExistsGetGetAllGetManyGetGroupedListPruneResetRenameRemoveRemoveManySearchSearchAndGetUpdateUpdateManyUpsertUpsertAll"

var _Op_index = [...]uint8{0, 3, 10, 15, 21, 27, 30, 36, 43, 53, 57, 62, 67, 73, 79, 89, 95, 107, 113, 123, 129, 138}

func (i Op) String() string {
	if i < 0 || i >= Op(len(_Op_index)-1) {
		return "Op(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Op_name[_Op_index[i]:_Op_index[i+1]]
}
