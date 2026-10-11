'use strict';

const e = React.createElement;

let WordSquare = ({valid, grid, constraintGrid}) => {
  let rows = [];
  for(let r = 0; r < 5; r++) {
	let cells = [];
	for(let c = 0; c < 5; c++) {
	  let classes = 'gridcell';
	  if(constraintGrid[r*5+c] != '') {
		let className = valid ? 'gridcell gridcell-constrained-valid' : 'gridcell gridcell-constrained-nosolution';
		cells.push(e('td', {key: c, className: className}, constraintGrid[r*5+c]));
	  } else if(valid) {
		cells.push(e('td', {key: c, className: 'gridcell'}, grid[r*5+c]));
	  } else {
		cells.push(e('td', {key: c, className: 'gridcell'}, '_'));
	  }
	}
	rows.push(e('tr', {key: r}, cells));
  }

  let className = valid ? 'wordsquare wordsquare-valid' : 'wordsquare wordsquare-nosolution';

  return e('div', {className: 'wordsquare-container'},
		   e('table', {className: className},
			 e('tbody', {}, rows)));
};

class ConstraintInput extends React.Component {
  constructor(props) {
	super(props);

	this.state = {
	  row: '',
	  col: '',
	  letter: '',
	};
  }

  changeRow(event) {
	this.setState({row: event.target.value});
  }

  changeCol(event) {
	this.setState({col: event.target.value});
  }

  changeLetter(event) {
	this.setState({letter: event.target.value});
  }

  isRowValid() {
	return !isNaN(parseInt(this.state.row, 10));
  }

  isColValid() {
	return !isNaN(parseInt(this.state.col, 10));
  }

  isLetterValid() {
	return this.state.letter.length == 1 && this.state.letter[0] >= 'a' && this.state.letter[0] <= 'z';
  }

  isStateValid() {
	return this.isRowValid() && this.isColValid() && this.isLetterValid();
  }

  addConstraint(event) {
	if(!this.isStateValid()) {
	  return;
	}
	this.props.onAddConstraint(
		parseInt(this.state.row, 10),
		parseInt(this.state.col, 10),
		this.state.letter,
	);

	event.preventDefault();
  }

  render() {
	return e('div', {},
			 e('h3', {}, 'New Constraint:'),
			 e('form', {onSubmit: (e) => this.addConstraint(e)},
			   e('label', {}, 'Row', e('input', {type: 'text', value: this.state.row, onChange: this.changeRow.bind(this)})),
			   e('label', {}, 'Col', e('input', {type: 'text', value: this.state.col, onChange: this.changeCol.bind(this)})),
			   e('label', {}, 'Letter', e('input', {type: 'text', value: this.state.letter, onChange: this.changeLetter.bind(this)})),
			   e('button', {type: 'submit'}, 'Add Constraint')));
  }
}

let Constraint = ({row, col, letter}) => {
  return e('div', {}, `(${row}, ${col}) → ${letter}`);
};


class ConstraintList extends React.Component {
  constructor(props) {
	super(props);
  }

  onDeleteConstraint(event, i) {
	this.props.onDeleteConstraint(i);
  }

  render() {
	let constraints = this.props.constraints;
	if(constraints.length === 0) {
	  return e(React.Fragment, {}, e('h3', {}, 'Constraints:'), '(none)');
	}

	let items = constraints.map((x, i) => (
		e('li', {key: i},
		  e(Constraint, {row: x.row,
						 col: x.col,
						 letter: x.letter}),
		  e('button', {onClick: (e) => this.onDeleteConstraint(e, i)},
			'Delete'))));

	return e(React.Fragment, {},
			 e('h3', {}, 'Constraints:'),
			 e('ul', {}, items));
  }
}

let constraintsEqual = (oldc, newc) => {
  if(oldc.length != newc.length) {
	return false;
  }

  for(let i = 0; i < oldc.length; i++) {
	if(oldc[i].row != newc[i].row) {
	  return false;
	}
	if(oldc[i].col != newc[i].col) {
	  return false;
	}
	if(oldc[i].letter != newc[i].letter) {
	  return false;
	}
  }

  return true;
};

class UI extends React.Component {
  constructor(props) {
	super(props);

	this.state = {
	  valid: false,
	  grid: [
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
	  ],
	  constraintGrid: [
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x',
	  ],
	  constraints: [],
	};
  }

  async componentDidMount() {
	await this.runEvaluate();
  }

  async componentDidUpdate(prevProps, prevState) {
	if(!constraintsEqual(prevState.constraints, this.state.constraints)) {
	  await this.runEvaluate();
	}
  }

  async runEvaluate() {
	const request = {
	  Constraints: this.state.constraints.map(x => ({Row: x.row, Col: x.col, Letter: x.letter})),
	};

	const response = await fetch("evaluate", {
	  method: 'POST',
	  headers: {'Content-Type': 'application/json'},
	  body: JSON.stringify(request),
	});

	if(response.status != 200) {
	  this.setState({valid: false})
	  return
	}

	const responseObj = await response.json();

	if(!responseObj.FoundSolution) {
	  this.setState({
		valid: false,
	    constraintGrid: responseObj.ConstraintGrid,
	  });
	  return;
	}

	this.setState({
	  valid: true,
	  grid: responseObj.Grid,
	  constraintGrid: responseObj.ConstraintGrid,
	});
  }

  deleteConstraint(i) {
	let newConstraints = Array.from(this.state.constraints);
	newConstraints.splice(i, 1);
	this.setState({constraints: newConstraints});
  }

  render() {
	return e(React.Fragment, {},
			 e(WordSquare, {valid: this.state.valid, grid: this.state.grid, constraintGrid: this.state.constraintGrid}),
			 e(ConstraintList, {constraints: this.state.constraints,
								onDeleteConstraint: (i) => this.deleteConstraint(i)}),
			 e(ConstraintInput, {
			   onAddConstraint: (row, col, letter) => {
				 this.setState((state, props) => ({
				   constraints: [...state.constraints, {row: row, col: col, letter: letter}],
				 }));
			   },
			 })
			);
  }
}

const domContainer = document.querySelector('#ui-container');
ReactDOM.render(e(UI), domContainer);
