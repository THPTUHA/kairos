import { useEffect, useState } from "react";
import { useRecoilState } from "recoil";
import queryBuildAtom from "../recoil/queryBuild/atom";
import { CopyToClipboard } from 'react-copy-to-clipboard';
import QueryableStyle from '../styles/Queryable.module.sass';
import { IoMdAddCircleOutline } from "react-icons/io";

interface Props {
    query: string,
    style?: unknown,
    iconStyle?: unknown,
    className?: string,
    useTooltip?: boolean,
    tooltipStyle?: unknown,
    displayIconOnMouseOver?: boolean,
    flipped?: boolean,
    children: React.ReactElement
  }
  
  const Queryable: React.FC<Props> = ({ query, style, iconStyle, className, useTooltip = true, tooltipStyle = null, displayIconOnMouseOver = false, flipped = false, children }) => {
    const [showAddedNotification, setAdded] = useState(false);
    const [showTooltip, setShowTooltip] = useState(false);
    const [queryBuild, setQuerytemp] = useRecoilState(queryBuildAtom);
  
    const onCopy = () => {
      setAdded(true)
    };
  
    useEffect(() => {
      let timer:any;
      if (showAddedNotification) {
        setQuerytemp(queryBuild ? `${queryBuild} and ${query}` : query);
        timer = setTimeout(() => {
          setAdded(false);
        }, 1000);
      }
      return () => clearTimeout(timer);
  
      // eslint-disable-next-line
    }, [showAddedNotification, query, setQuerytemp]);
  
    const addButton = query ? <CopyToClipboard text={query} onCopy={onCopy}>
      <span
        className={QueryableStyle.QueryableIcon}
        title={`Add "${query}" to the filter`}
        //@ts-ignore
        style={iconStyle}>
        <IoMdAddCircleOutline fontSize="small" color="inherit" />
        {showAddedNotification && <span className={QueryableStyle.QueryableAddNotifier}>Added</span>}
      </span>
    </CopyToClipboard> : null;
  
    return (
      <div className={`${QueryableStyle.QueryableContainer} ${QueryableStyle.displayIconOnMouseOver} ${className ? className : ''} ${displayIconOnMouseOver ? QueryableStyle.displayIconOnMouseOver : ''}`}
       //@ts-ignore
       style={style} 
       onMouseOver={() => setShowTooltip(true)} onMouseLeave={() => setShowTooltip(false)}>
        {flipped && addButton}
        {children}
        {!flipped && addButton}
        {useTooltip && showTooltip && (query !== "") && 
        //@ts-ignore
        <span data-cy={"QueryableTooltip"} className={QueryableStyle.QueryableTooltip} style={tooltipStyle}>{query}</span>}
      </div>
    );
  };
  
  export default Queryable;
  