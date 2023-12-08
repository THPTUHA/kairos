package agent

import (
	"strconv"
	"strings"

	"github.com/hashicorp/serf/serf"
	"github.com/sirupsen/logrus"
)

func cleanTags(tags map[string]string, logger *logrus.Entry) (map[string]string, int) {
	cardinality := int(^uint(0) >> 1)

	cleanTags := make(map[string]string, len(tags))

	for k, v := range tags {
		vparts := strings.Split(v, ":")

		cleanTags[k] = vparts[0]
		if len(vparts) == 2 {
			tagCard, err := strconv.Atoi(vparts[1])
			if err != nil {
				tagCard = 0
				logger.Errorf("improper cardinality specified for tag %s: %v", k, vparts[1])
			}

			if tagCard < cardinality {
				cardinality = tagCard
			}
		}
	}

	return cleanTags, cardinality
}

func nodeMatchesTags(node serf.Member, tags map[string]string) bool {
	for k, v := range tags {
		nodeVal, present := node.Tags[k]
		if !present {
			return false
		}
		if nodeVal != v {
			return false
		}
	}
	return true
}
