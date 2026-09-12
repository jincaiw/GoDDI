package ipam

// A list in a response is an array, and an array is never null.
//
// A nil slice marshals to `null`, which is not the same answer as `[]`: null
// reads as "the field is absent" and `[]` reads as "there are none". The
// console counts these lists to decide what to render, so the difference is not
// academic -- `"conflicts": null` was a TypeError inside a Vue render, and the
// operator saw a drawer with a title and no body, and an import dialog that
// closed itself instead of showing a report.
//
// DHCPScopePlan has written its lists as `[]T{}` since it was added. The view
// and the import report are assembled from helpers that return nil when they
// find nothing, so those lists are wrapped on the way out instead.
//
// Every list on the three response types is covered by
// TestNoResponseBuilderLeavesAListNil, which reflects over the structs rather
// than naming the fields. Naming them is what failed here: the fields someone
// remembered to check were the ones already spelled `[]T{}`.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
