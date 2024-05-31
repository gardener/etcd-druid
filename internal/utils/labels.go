package utils

// ContainsAllDesiredLabels checks if the actual map contains all the desired labels.
func ContainsAllDesiredLabels(actual, desired map[string]string) bool {
	for key, desiredValue := range desired {
		actualValue, ok := actual[key]
		if !ok || actualValue != desiredValue {
			return false
		}
	}
	return true
}

// ContainsLabel checks if the actual map contains the specified key-value pair.
func ContainsLabel(actual map[string]string, key, value string) bool {
	actualValue, ok := actual[key]
	return ok && actualValue == value
}
