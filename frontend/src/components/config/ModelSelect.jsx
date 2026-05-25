export function ModelSelect({ models, value, onChange, disabled }) {
  return (
    <select className="onb-mselect" value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled}>
      {disabled && !models.length && <option>验证后可选</option>}
      {models.map((m) => <option key={m} value={m}>{m}</option>)}
    </select>
  );
}
