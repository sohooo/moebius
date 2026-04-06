package validate

func statusRank(status Status) int {
	switch status {
	case StatusError:
		return 3
	case StatusWarning:
		return 2
	default:
		return 1
	}
}
