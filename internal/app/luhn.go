package app

func IsValidLuhn(number string) bool {
	digitsCount := len(number)
	isSecond := false
	sum := 0

	for i := digitsCount - 1; i >= 0; i-- {
		d := number[i] - '0'
		if isSecond {
			d = d * 2
		}

		sum += int(d) / 10
		sum += int(d) % 10

		isSecond = !isSecond
	}

	return sum%10 == 0
}
