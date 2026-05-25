import { Icon } from "../primitives/Icon.jsx";

export function AppearanceSection({ open, onToggle }) {
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Brush className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">外观</div>
          <div className="set-sec-t2">主题 · 主题色 · 密度 · 语言</div>
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
