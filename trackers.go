package replica

func Trackers() []string {
	return []string{
		"https://tracker.gbitt.info:443/announce",
		"http://tracker.opentrackr.org:1337/announce",
		"udp://tracker.leechers-paradise.org:6969/announce",
	}
}
