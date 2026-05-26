import { Icon } from "@shared/ui/Icon";

export function KeyVerifyField({ label, value, onChange, onVerify, verifying, verified, error, verifyLabel, verifyingLabel, verifiedLabel, placeholder, readOnly }: { label?: any; value?: any; onChange?: any; onVerify?: any; verifying?: any; verified?: any; error?: any; verifyLabel?: any; verifyingLabel?: any; verifiedLabel?: any; placeholder?: any; readOnly?: any }) {
  return (
    <>
      <div className="onb-klabel">{label}</div>
      <div className={"onb-kinput" + (error ? " is-error" : "")}>
        <Icon.KeyRound />
        {readOnly
          ? <input value={value} readOnly style={{ color: "var(--fg-faint)" }} />
          : <input type="password" placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} autoFocus />}
        {verified ? (
          <span className="onb-verified"><Icon.Check /> {verifiedLabel}</span>
        ) : (
          <button type="button" className="onb-verify-btn" onClick={onVerify} disabled={verifying || (!readOnly && !value?.trim())}>
            {verifying ? verifyingLabel : verifyLabel}
          </button>
        )}
      </div>
      {error && <div className="onb-verify-err"><Icon.AlertCircle /> {error}</div>}
    </>
  );
}
