package agent

import "github.com/hashicorp/serf/serf"

// TODO
// nodeJoin is used to handle join events on the serf cluster
func (a *Agent) nodeJoin(me serf.MemberEvent) {

}

// TODO
// localMemberEvent is used to reconcile Serf events with the
// consistent store if we are the current leader.
func (a *Agent) localMemberEvent(me serf.MemberEvent) {

}

// TODO
// nodeFailed is used to handle fail events on the serf cluster
func (a *Agent) nodeFailed(me serf.MemberEvent) {

}
