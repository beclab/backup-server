package controllers

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func newCreateOnlyPredicate(createFn func(e event.CreateEvent) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			if createFn != nil {
				return createFn(createEvent)
			}
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}

func newCreateDeletePredicate(createFn func(e event.CreateEvent) bool, deleteFn func(e event.DeleteEvent) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			if createFn != nil {
				return createFn(createEvent)
			}
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			if deleteFn != nil {
				return deleteFn(deleteEvent)
			}
			return true
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}

func newDeleteOnlyPredicate(deleteFn func(e event.DeleteEvent) bool) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			if deleteFn != nil {
				return deleteFn(deleteEvent)
			}
			return true
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
}
