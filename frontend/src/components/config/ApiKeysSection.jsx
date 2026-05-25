import { Icon } from "../primitives/Icon.jsx";

export function ApiKeysSection({ open, onToggle }) {
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.KeyRound className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">API Keys</div>
          <div className="set-sec-t2">管理 LLM 服务密钥</div>
        </div>
        <Icon.ChevronRight
          className={"set-sec-chev icon" + (open ? " is-open" : "")}
        />
      </button>
      {open && (
        <div className="set-sec-p">
          <div className="set-sec-empty">即将实现</div>
        </div>
      )}
    </div>
  );
}
