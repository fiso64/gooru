package gooru

import "testing"

func TestBackgroundOperationChangesCoalesceAndUnsubscribe(t *testing.T) {
	client := &Client{}
	changes, unsubscribe := client.SubscribeBackgroundOperationChanges()

	client.notifyBackgroundOperationChange()
	client.notifyBackgroundOperationChange()

	select {
	case <-changes:
	default:
		t.Fatal("expected committed-operation change signal")
	}
	select {
	case <-changes:
		t.Fatal("duplicate signals should coalesce until the subscriber catches up")
	default:
	}

	unsubscribe()
	if _, ok := <-changes; ok {
		t.Fatal("unsubscribe must close the subscriber channel")
	}
	client.notifyBackgroundOperationChange()
}

func TestBackgroundOperationChangesAreClientScoped(t *testing.T) {
	first := &Client{}
	second := &Client{}
	firstChanges, unsubscribeFirst := first.SubscribeBackgroundOperationChanges()
	defer unsubscribeFirst()
	secondChanges, unsubscribeSecond := second.SubscribeBackgroundOperationChanges()
	defer unsubscribeSecond()

	first.notifyBackgroundOperationChange()
	select {
	case <-firstChanges:
	default:
		t.Fatal("expected signal for first client")
	}
	select {
	case <-secondChanges:
		t.Fatal("operation changes must not cross client instances")
	default:
	}
}
