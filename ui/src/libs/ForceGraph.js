import React, { Component } from "react";
import Graph from "react-graph-vis";

let ctx;
let network;
let container;
let canvas;
let nodes;
let edges;
let rect = [];
var drawingSurfaceImageData;

class ForceGraph extends Component {
  constructor(props) {
    super(props);
    this.state = {
      drag: false,
    };
    this.canvasWrapperRef = React.createRef();
  }

  componentDidMount() {
    container = this.canvasWrapperRef.current.Network.canvas.frame;
    network = this.canvasWrapperRef.current.Network;
    canvas = this.canvasWrapperRef.current.Network.canvas.frame.canvas;
    nodes = this.props.graph.nodes;
    edges = this.props.graph.edges;
    ctx = canvas.getContext("2d");

    container.oncontextmenu = function() {
      return false;
    };

    this.saveDrawingSurface();

    // add event watching for canvas box drawing
    container.addEventListener("mousedown", e => {
      e.preventDefault();

      if (e.button == 2) {
        this.saveDrawingSurface();
        rect.startX =
          e.pageX -
          this.canvasWrapperRef.current.Network.body.container.offsetLeft -
          this.props.modalRef.current.getBoundingClientRect().left;
        rect.startY =
          e.pageY -
          this.canvasWrapperRef.current.Network.body.container.offsetTop -
          this.props.modalRef.current.getBoundingClientRect().top;
        this.state.drag = true;
        container.style.cursor = "crosshair";
        this.setSelectionFromHighlight();
      }
    });

    container.addEventListener("mousemove", e => {
      if (this.state.drag) {
        this.restoreDrawingSurface();
        rect.w =
          e.pageX -
          this.canvasWrapperRef.current.Network.body.container.offsetLeft - this.props.modalRef.current.getBoundingClientRect().left -
          rect.startX;
        rect.h =
          e.pageY -
          this.canvasWrapperRef.current.Network.body.container.offsetTop - this.props.modalRef.current.getBoundingClientRect().top -
          rect.startY;

        ctx.setLineDash([5]);
        ctx.strokeStyle = "rgb(0, 102, 0)";
        ctx.strokeRect(rect.startX, rect.startY, rect.w, rect.h);
        ctx.setLineDash([]);
        ctx.fillStyle = "rgba(0, 255, 0, 0.2)";
        ctx.fillRect(rect.startX, rect.startY, rect.w, rect.h);
      }
    });

    container.addEventListener("mouseup", e => {
      if (e.button == 2) {
        this.restoreDrawingSurface();
        this.state.drag = false;
        container.style.cursor = "default";
        this.setSelectionFromHighlight();
      }
    });
  }

  componentDidUpdate() {
    try {
      network.setSelection(this.props.selection);
    } catch (error) {
    }
  }

  saveDrawingSurface() {
    drawingSurfaceImageData = ctx.getImageData(
      0,
      0,
      canvas.width,
      canvas.height
    );
  }

  restoreDrawingSurface() {
    ctx.putImageData(drawingSurfaceImageData, 0, 0);
  }

  selectItemsFromHighlight(items) {
    var itemsIdInDrawing = [];
    var xRange = this.getStartToEnd(rect.startX, rect.w);
    var yRange = this.getStartToEnd(rect.startY, rect.h);
    var allItems = items;
    for (var i = 0; i < allItems.length; i++) {
      var curItem = allItems[i];
      var itemPosition = network.getPositions([curItem.id]);
      if (!itemPosition[curItem.id]) continue;

      var itemXY = network.canvasToDOM({
        x: itemPosition[curItem.id].x,
        y: itemPosition[curItem.id].y
      });
      if (
        xRange.start <= itemXY.x &&
        itemXY.x <= xRange.end &&
        yRange.start <= itemXY.y &&
        itemXY.y <= yRange.end
      ) {
        itemsIdInDrawing.push(curItem.id);
      }
    }

    return itemsIdInDrawing;
  }

  setSelectionFromHighlight() {
    var nodesIdInDrawing = this.selectItemsFromHighlight(nodes);
    var edgesIdInDrawing = this.selectItemsFromHighlight(edges);
    network.setSelection({
      nodes: nodesIdInDrawing,
      edges: edgesIdInDrawing
    });
    this.props.setSelectedNodes(nodesIdInDrawing);
    this.props.setSelectedEdges(edgesIdInDrawing);
  }

  getStartToEnd(start, theLen) {
    return theLen > 0
      ? {
          start: start,
          end: start + theLen
        }
      : {
          start: start + theLen,
          end: start
        };
  }

  render() {
    return (
      <div
        id="graph"
        style={{ height: "100%", width: "100%" }}
      >
        <Graph
          ref={this.canvasWrapperRef}
          graph={this.props.graph}
          options={this.props.options}
          events={this.props.events}
          getNetwork={this.props.getNetwork}
        />
      </div>
    );
  }
}

export default ForceGraph;
