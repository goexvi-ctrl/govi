//go:build !unix

package tcell

func saveEmergencyTermios() {}

func installEmergencyHandlers() {}