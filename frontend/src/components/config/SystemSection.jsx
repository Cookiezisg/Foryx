import { Icon } from "../primitives/Icon.jsx";

export function SystemSection({ open, onToggle }) {
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Server className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">系统</div>
          <div className="set-sec-t2">本地存储 · 内置运行时</div>
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
