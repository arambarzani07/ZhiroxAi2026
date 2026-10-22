// Isolated pre-deployment guard. Integrate only after the migration is applied.
export function validateItemNameDeltaEvent(eventType, payload) {
  if (eventType !== "itemname_delta") return { ok: true, eventKey: null };
  if (!payload || typeof payload !== "object" || Array.isArray(payload))
    return { ok: false, error: "invalid_itemname_payload" };
  const eventKey = payload.event_key;
  if (typeof eventKey !== "string" || !/^[a-f0-9]{64}$/.test(eventKey))
    return { ok: false, error: "invalid_event_key" };
  if (payload.database !== "SuperMarket" || payload.read_only !== true)
    return { ok: false, error: "invalid_itemname_scope" };
  if (!Array.isArray(payload.changes) || payload.changes.length < 1 || payload.changes.length > 25)
    return { ok: false, error: "invalid_change_batch" };
  for (const item of payload.changes) {
    if (!item || typeof item !== "object" || Array.isArray(item))
      return { ok: false, error: "invalid_change_item" };
    if (typeof item.itemNameID !== "string" || !item.itemNameID.trim() || item.itemNameID.length > 50)
      return { ok: false, error: "invalid_item_id" };
    if (item.kind !== "added" && item.kind !== "price_changed")
      return { ok: false, error: "invalid_change_kind" };
    if (item.kind === "price_changed" &&
      (item.priceField !== "SalePrice" && item.priceField !== "WholeSalePrice"))
      return { ok: false, error: "invalid_price_field" };
  }
  return { ok: true, eventKey };
}

export function classifyItemNameInsertResult(error) {
  if (!error) return "inserted";
  if (error.code === "23505" &&
      String(error.message || "").includes("pc_os_events_agent_event_key_unique"))
    return "duplicate";
  return "error";
}
