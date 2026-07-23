export function investigationDisplayId(inv: {
  investigation_number?: number;
  investigation_id?: string;
  alert_investigation_number?: number;
  alert_investigation_id?: string;
}): string {
  if (inv.alert_investigation_number != null && inv.alert_investigation_number > 0)
    return `AINV-${inv.alert_investigation_number}`;
  if (inv.investigation_number != null && inv.investigation_number > 0)
    return `INV-${inv.investigation_number}`;
  const id = inv.alert_investigation_id ?? inv.investigation_id ?? "";
  return id ? `${id.slice(0, 8)}` : "";
}
