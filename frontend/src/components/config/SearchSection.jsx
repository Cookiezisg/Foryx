import { Icon } from "../primitives/Icon.jsx";

export function SearchSection({ open, onToggle }) {
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Search className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">
            网络搜索
            <span className="set-sec-opt-tag">可选</span>
          </div>
          <div className="set-sec-t2">博查 · Brave · Serper · Tavily</div>
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
