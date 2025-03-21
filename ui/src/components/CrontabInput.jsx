import React, { Component } from 'react';
import PropTypes from 'prop-types';
import Cron from "cron-converter";
import cronstrue from 'cronstrue/i18n';
import en from "./en";
const valueHints = { en }
class CrontabInput extends Component {
  state = {
    parsed: {},
    highlightedExplanation: "",
    isValid: true,
    selectedPartIndex: -1,
    nextSchedules: [],
    nextExpanded: false,
  };
  inputRef;

  lastCaretPosition = -1;

  componentWillMount() {
    this.calculateNext();
    this.calculateExplanation();
  }

  clearCaretPosition() {
    this.lastCaretPosition = -1;
    this.setState({
      selectedPartIndex: -1,
      highlightedExplanation: this.highlightParsed(-1),
    });
  }

  calculateNext() {
    let nextSchedules = [];
    try {
      let cronInstance = new Cron();
      cronInstance.fromString(this.props.value);
      let timePointer = +new Date();
      for (let i = 0; i < 5; i++) {
        let schedule = cronInstance.schedule(timePointer);
        let next = schedule.next();
        nextSchedules.push(next.format("YYYY-MM-DD HH:mm:ss"));
        timePointer = +next + 1000;
        if(nextSchedules.length){
          this.setState({
            isValid: true,
          })
        }else{
          this.setState({
            isValid: false,
          })
        }
      }
    } catch (e) {
      console.error(e)
    }
    this.setState({ nextSchedules });
  }

  highlightParsed(selectedPartIndex) {
    let parsed = this.state.parsed;
    if (!parsed) {
      return;
    }
    let toHighlight = [];
    let highlighted = "";
    if(!parsed.segments){
      return []
    }
    for (let i = 0; i < 5; i++) {
      if (parsed.segments[i] && parsed.segments[i].text) {
        toHighlight.push({...parsed.segments[i]});
      } else {
        toHighlight.push(null);
      }
    }

    if (selectedPartIndex >= 0) {
      if (toHighlight[selectedPartIndex]) {
        toHighlight[selectedPartIndex].active = true;
      }
    }


    if (toHighlight[0] && toHighlight[1] && toHighlight[0].text && toHighlight[0].text === toHighlight[1].text && toHighlight[0].start === toHighlight[1].start) {
      if (toHighlight[1].active) {
        toHighlight[0] = null;
      } else {
        toHighlight[1] = null;
      }
    }

    toHighlight = toHighlight.filter(_ => _);

    toHighlight.sort((a, b) => {
      return a.start - b.start;
    });
    
    let pointer = 0;
    toHighlight.forEach(item => {
      if (pointer > item.start) {
        return;
      }
      highlighted += parsed.description.substring(pointer, item.start);
      pointer = item.start;
      highlighted += `<span${item.active ? ' class="active"' : ''}>${parsed.description.substring(pointer, pointer + item.text.length)}</span>`;
      pointer += item.text.length;
    });

    highlighted += parsed.description.substring(pointer);

    return highlighted;
  }

  calculateExplanation(callback) {
    let isValid = true;
    let parsed;
    let highlightedExplanation;
    // try {
    //   parsed = cronstrue.parse(this.props.value, { locale: "en" });
    //   highlightedExplanation = "";

    // } catch (e) {
    //   console.error(e)
    //   highlightedExplanation = e.toString();
    // }
    this.setState({
      parsed,
      highlightedExplanation,
      isValid,
    }, () => {
      if (isValid) {
        this.setState({
          highlightedExplanation: this.highlightParsed(-1),
        });
      }
      if (callback) {
        callback();
      }
    });
  }

  onCaretPositionChange() {
    if (!this.inputRef) {
      return;
    }
    let caretPosition = this.inputRef.selectionStart;
    let selected = this.props.value.substring(this.inputRef.selectionStart, this.inputRef.selectionEnd);
    if (selected.indexOf(" ") >= 0) {
      caretPosition = -1;
    }
    if (this.lastCaretPosition === caretPosition) {
      return;
    }
    this.lastCaretPosition = caretPosition;
    if (caretPosition === -1) {
      this.setState({
        highlightedExplanation: this.highlightParsed(-1),
        selectedPartIndex: -1,
      });
      return;
    }
    let textBeforeCaret = this.props.value.substring(0, caretPosition);
    let selectedPartIndex = textBeforeCaret.split(" ").length - 1;
    this.setState({
      highlightedExplanation: this.highlightParsed(selectedPartIndex),
      selectedPartIndex,
    });
  }

  getLocale() {
    return {
      nextTime:"next",
      then:"then",
      showMore:"show more",
      hide:"hide",
      minute: "minute",
      hour: "hour",
      dayMonth: "day(month)",
      month: "month",
      dayWeek: "day(week)",
    };
  }

  componentWillReceiveProps(nextProps) {
    console.log(this.props.value,"---",nextProps.value )
    if (nextProps.value !== this.props.value) {
      setTimeout(() => {
        this.calculateExplanation(() => {
          this.onCaretPositionChange();
          this.calculateNext();
        });
      });
    }
  }

  render() {
    return (
      <div className="crontab-input">
        

        <div className="next">
          {!!this.state.nextSchedules.length && <span>
            {this.getLocale().nextTime}: {this.state.nextSchedules[0]} {this.state.nextExpanded ?
            <a onClick={() => this.setState({ nextExpanded: false })}>({this.getLocale().hide})</a> :
            <a onClick={() => this.setState({ nextExpanded: true })}>({this.getLocale().showMore})</a>}
            {!!this.state.nextExpanded && <div className="next-items">
              {this.state.nextSchedules.slice(1).map((item, index) => <div
                className="next-item" key={index}>{this.getLocale().then}: {item}</div>)}
            </div>}
          </span>}
        </div>

        <input type="text" className={"cron-input " + (!this.state.isValid ? "error" : "primary")}
               value={this.props.value}
               ref={ref => {
                 this.inputRef = ref;
               }}
               onMouseUp={e => {
                 this.onCaretPositionChange()
               }}
               onKeyUp={e => {
                 this.onCaretPositionChange()
               }}
               onBlur={e => {
                 this.clearCaretPosition()
               }}
               onChange={e => {
                 let parts = e.target.value.split(" ").filter(_ => _);
                 if (parts.length !== 5) {
                   this.props.onChange(e.target.value);
                   this.setState({
                     parsed: {},
                     isValid: false,
                   });
                   return;
                 }

                 this.props.onChange(e.target.value);
               }}/>


        <div className="parts">
          {[
            this.getLocale().minute,
            this.getLocale().hour,
            this.getLocale().dayMonth,
            this.getLocale().month,
            this.getLocale().dayWeek,
          ].map((unit, index) => (
            <div key={index}
                 className={"part " + (this.state.selectedPartIndex === index ? "selected" : "")}>{unit}</div>
          ))}
        </div>

        {valueHints["en"][this.state.selectedPartIndex] && <div className="allowed-values">
          {valueHints["en"][this.state.selectedPartIndex].map((value, index) => (
            <div className="value" key={index}>
              <div className="key">{value[0]}</div>
              <div className="value">{value[1]}</div>
            </div>
          ))}
        </div>}

      </div>
    );
  }
}

CrontabInput.propTypes = {
  locale: PropTypes.string,
  value: PropTypes.string,
  onChange: PropTypes.func,
};

CrontabInput.defaultProps = {
  locale: "en",
};

export default CrontabInput;
