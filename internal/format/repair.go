package format

// Repair encoding is deliberately private: its AST may contain missing IDs and
// must never be accepted by the normal, validated document encoder.
type encodeRepairRequest struct{ Document RepairDocument }

func encodeRepair(r encodeRepairRequest) []byte {
	return encodeThreads(encodeThreadsRequest{Threads: r.Document.Threads})
}
