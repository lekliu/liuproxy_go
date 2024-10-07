package string

import "strconv"

func StrToInt(data string, defaultInt int) int {
	retData, err := strconv.Atoi(data)
	if err != nil {
		retData = defaultInt
	}
	return retData
}
