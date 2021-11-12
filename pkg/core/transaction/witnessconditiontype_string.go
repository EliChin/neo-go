// Code generated by "stringer -type=WitnessConditionType -linecomment"; DO NOT EDIT.

package transaction

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[WitnessBoolean-0]
	_ = x[WitnessNot-1]
	_ = x[WitnessAnd-2]
	_ = x[WitnessOr-3]
	_ = x[WitnessScriptHash-24]
	_ = x[WitnessGroup-25]
	_ = x[WitnessCalledByEntry-32]
	_ = x[WitnessCalledByContract-40]
	_ = x[WitnessCalledByGroup-41]
}

const (
	_WitnessConditionType_name_0 = "BooleanNotAndOr"
	_WitnessConditionType_name_1 = "ScriptHashGroup"
	_WitnessConditionType_name_2 = "CalledByEntry"
	_WitnessConditionType_name_3 = "CalledByContractCalledByGroup"
)

var (
	_WitnessConditionType_index_0 = [...]uint8{0, 7, 10, 13, 15}
	_WitnessConditionType_index_1 = [...]uint8{0, 10, 15}
	_WitnessConditionType_index_3 = [...]uint8{0, 16, 29}
)

func (i WitnessConditionType) String() string {
	switch {
	case 0 <= i && i <= 3:
		return _WitnessConditionType_name_0[_WitnessConditionType_index_0[i]:_WitnessConditionType_index_0[i+1]]
	case 24 <= i && i <= 25:
		i -= 24
		return _WitnessConditionType_name_1[_WitnessConditionType_index_1[i]:_WitnessConditionType_index_1[i+1]]
	case i == 32:
		return _WitnessConditionType_name_2
	case 40 <= i && i <= 41:
		i -= 40
		return _WitnessConditionType_name_3[_WitnessConditionType_index_3[i]:_WitnessConditionType_index_3[i+1]]
	default:
		return "WitnessConditionType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
}
