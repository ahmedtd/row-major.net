'use strict';

const doNothingVertexShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

in vec2 vertexPos;
void main() {
    gl_Position = vec4(vertexPos, 0.0, 1.0);
}
`;

const boundaryConditionFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform vec2 stirrerPos;
uniform float stirrerRadius;

uniform sampler2D oldVelocity;

out vec2 newVelocity;

void main() {
  ivec2 velocitySize = textureSize(oldVelocity, 0);

  // In this shader, our fragment coordinates are cell coordinates.
  vec2 coord = gl_FragCoord.xy;
  ivec2 cell = ivec2(floor(coord));
  vec2 cellCenter = vec2(cell) + vec2(0.5, 0.5);

  if(cell.x == 0 && cell.y == 0) {
    newVelocity = vec2(0.0, 0.0);
    return;
  }
  if(cell.x == 0 && cell.y == velocitySize.y-1) {
    newVelocity = vec2(0.0, 0.0);
    return;
  }
  if(cell.x == velocitySize.x-1 && cell.y == 0) {
    newVelocity = vec2(0.0, 0.0);
    return;
  }
  if(cell.x == velocitySize.x-1 && cell.y == velocitySize.y-1) {
    newVelocity = vec2(0.0, 0.0);
    return;
  }

  // Left edge.
  if(cell.x == 0) {
    newVelocity.x = 0.0;
    newVelocity.y = texelFetch(oldVelocity, cell+ivec2(1,0), 0).y;
    return;
  }

  // Right edge.
  if(cell.x == velocitySize.x-1) {
    newVelocity.x = 0.0;
    newVelocity.y = texelFetch(oldVelocity, cell+ivec2(-1,0), 0).y;
    return;
  }

  // Bottom edge.
  if(cell.y == 0) {
    newVelocity.x = texelFetch(oldVelocity, cell+ivec2(0,1), 0).x;
    newVelocity.y = 0.0;
    return;
  }

  // Top edge.
  if(cell.y == velocitySize.y-1) {
    newVelocity.x = texelFetch(oldVelocity, cell+ivec2(0,-1), 0).x;
    newVelocity.y = 0.0;
    return;
  }

  if(length(cellCenter - stirrerPos) < stirrerRadius) {
    newVelocity.x = 0.0;
    newVelocity.y = 5.0;
    return;
  }

  newVelocity = texelFetch(oldVelocity, ivec2(cell), 0).xy;
}

`

const advectFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform float dt;
uniform float gridScale;
uniform sampler2D oldVelocity;

out vec2 newVelocity;

void main() {
  vec2 velocitySize = vec2(textureSize(oldVelocity, 0));

  // In this shader, our fragment coordinates are cell coordinates.
  vec2 coord = gl_FragCoord.xy;
  vec2 cell = floor(coord);
  vec2 cellCenter = floor(cell) + vec2(0.5, 0.5);

  vec2 velocity = texelFetch(oldVelocity, ivec2(cell), 0).xy;

  // Make sure we don't process boundary cells.
  if(coord.x < 1.0 ||
     coord.x >= velocitySize.x - 1.0 ||
     coord.y < 1.0 ||
     coord.y > velocitySize.y - 1.0
  ) {
    newVelocity = velocity;
    return;
  }

  // Compute origin point for the packet currently at the center of this cell.
  vec2 origin = cellCenter - velocity * dt / gridScale;

  // Clamp to non-boundary cells; fluid can't cross the grid boundary.
  origin = clamp(origin, vec2(1.0,1.0), velocitySize - vec2(1.0,1.0));

  ivec2 originCell = ivec2(floor(origin));

  // Compute velocities at the four corners of the origin cell.
  vec2 velTL = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,1), 0).xy
  );
  vec2 velTR = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,1), 0).xy
  );
  vec2 velBL = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,-1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(-1,-1), 0).xy
  );
  vec2 velBR = 0.25 * (
    texelFetch(oldVelocity, originCell, 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,0), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(0,-1), 0).xy +
    texelFetch(oldVelocity, originCell+ivec2(1,-1), 0).xy
  );

  // Interpolate between the cell corners to compute the new velocity.
  vec2 s = origin - vec2(originCell);

  newVelocity =
    (1.0-s.x) * (1.0-s.y) * velBL +
    s.x * (1.0-s.y) * velBR +
    (1.0-s.x) * s.y * velTL +
    s.x * s.y * velTR;
}
`;

const computeDivergenceFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform sampler2D oldVelocity;

out float divergence;

void main() {
  vec2 velocitySize = vec2(textureSize(oldVelocity, 0));

  // In this shader, our fragment coordinates are cell coordinates.
  vec2 coord = gl_FragCoord.xy;
  vec2 cell = floor(coord);
  vec2 cellCenter = floor(cell) + vec2(0.5, 0.5);

  // Make sure we don't process boundary cells.
  if(coord.x < 1.0 ||
     coord.x >= velocitySize.x - 1.0 ||
     coord.y < 1.0 ||
     coord.y > velocitySize.y - 1.0
  ) {
    divergence = 0.0; // TODO(ahmedtd): Think about boundary conditions and put the correct value.
    return;
  }

  divergence =
    0.5 * (texelFetch(oldVelocity, ivec2(cell)+ivec2(1,0), 0).x - texelFetch(oldVelocity, ivec2(cell)+ivec2(-1,0), 0).x) +
    0.5 * (texelFetch(oldVelocity, ivec2(cell)+ivec2(0,1), 0).y - texelFetch(oldVelocity, ivec2(cell)+ivec2(0,-1), 0).y);
}
`

const removeDivergenceFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform sampler2D oldVelocity;
uniform sampler2D divergence;

out vec2 newVelocity;

void main() {
  vec2 velocitySize = vec2(textureSize(oldVelocity, 0));

  // In this shader, our fragment coordinates are cell coordinates.
  vec2 coord = gl_FragCoord.xy;
  vec2 cell = floor(coord);
  vec2 cellCenter = floor(cell) + vec2(0.5, 0.5);

  vec2 velocity = texelFetch(oldVelocity, ivec2(cell), 0).xy;

  // Make sure we don't process boundary cells.
  if(coord.x < 1.0 ||
     coord.x >= velocitySize.x - 1.0 ||
     coord.y < 1.0 ||
     coord.y > velocitySize.y - 1.0
  ) {
    newVelocity = velocity;
    return;
  }

  vec2 gradient = vec2(
    0.5 * (texelFetch(divergence, ivec2(cell)+ivec2(1,0), 0).x - texelFetch(divergence, ivec2(cell)+ivec2(-1,0), 0).x),
    0.5 * (texelFetch(divergence, ivec2(cell)+ivec2(0,1), 0).x - texelFetch(divergence, ivec2(cell)+ivec2(0,-1), 0).x)
  );

  newVelocity = velocity + 0.9 * gradient;
}
`

const renderFragmentShader = `#version 300 es
// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform ivec2 viewport;
uniform float metersPerCell;
uniform sampler2D oldVelocity;
uniform sampler2D divergence;

out vec4 fragColor;

float onNeedle(vec2 point, vec2 start, vec2 end, float widthBase, float aaBorder) {
  if(length(end - start) < 0.001) {
    return 0.0;
  }

  float lengthParallel = dot(normalize(end - start), point - start);
  vec2 parallel = lengthParallel * normalize(end - start);
  vec2 perpendicular = (point - start) - parallel;

  float t = lengthParallel / length(end - start);

  if(t < 0.0 || t > 1.0) {
    return 0.0;
  }

  float width = t * 0.0 + (1.0-t) * widthBase;

  if(length(perpendicular) < width - aaBorder) {
    return 1.0;
  } else if(length(perpendicular) < width + aaBorder) {
    float s = (length(perpendicular) - (width - aaBorder)) / ((width + aaBorder) - (width - aaBorder));
    return 1.0 - s;
  } else {
    return 0.0;
  }
}

void main() {
  ivec2 velocitySize = textureSize(oldVelocity, 0);
  vec2 pxPerGrid = vec2(viewport) / vec2(velocitySize);

  vec2 velocityGridFragCoord = gl_FragCoord.xy / pxPerGrid;

  float maxOnNeedle = 0.0;
  for(int dx = -3; dx <= 3; dx++) {
    for(int dy = -3; dy <= 3; dy++) {
      ivec2 cell;
      cell.x = int(velocityGridFragCoord.x) + dx;
      cell.y = int(velocityGridFragCoord.y) + dy;

      if(cell.x < 0 || cell.x >= velocitySize.x || cell.y < 0 || cell.y >= velocitySize.y) {
        continue;
      }

      vec2 velocity = texelFetch(oldVelocity, cell, 0).xy;
      vec2 cellCenter = vec2(cell) + vec2(0.5, 0.5);

      float curOnNeedle = onNeedle(
        gl_FragCoord.xy,
        cellCenter * pxPerGrid,
        (cellCenter + velocity / vec2(metersPerCell)) * pxPerGrid,
        1.0,
        0.5
      );

      if(curOnNeedle > maxOnNeedle) {
        maxOnNeedle = curOnNeedle;
      }
    }
  }

  vec2 divergenceSize = vec2(textureSize(divergence, 0));
  vec2 divergenceGridCoord = gl_FragCoord.xy / vec2(viewport) * divergenceSize;
  ivec2 divergenceCell = ivec2(divergenceGridCoord);
  float divergence = texelFetch(divergence, divergenceCell, 0).x;

  vec3 divergenceMinColor = vec3(0.0, 0.0, 1.0);
  vec3 divergence0Color = vec3(1.0, 1.0, 1.0);
  vec3 divergenceMaxColor = vec3(1.0, 0.0, 0.0);
  vec3 divergenceColor = vec3(0.0, 0.0, 0.0);
  if(divergence < 0.0) {
    divergenceColor = mix(divergenceMinColor, divergence0Color, clamp((divergence + 1.0) / 1.0, 0.0, 1.0));
  } else {
    divergenceColor = mix(divergence0Color, divergenceMaxColor, clamp(divergence / 1.0, 0.0, 1.0));
  }

  vec3 lineColor = vec3(0.0, 0.0, 0.0);

  fragColor.rgb = mix(divergenceColor, lineColor, maxOnNeedle);
  fragColor.a = 1.0;
}
`;

function compileShaderObject(gl, type, text) {
  let shaderObject = gl.createShader(type);
  gl.shaderSource(shaderObject, text);
  gl.compileShader(shaderObject);

  if(!gl.getShaderParameter(shaderObject, gl.COMPILE_STATUS)) {
    throw new Error("Error compiling shader: " + gl.getShaderInfoLog(shaderObject));
  }

  return shaderObject;
}

function buildShaderProgram(gl, vertexShaders, fragmentShaders) {
  let vertexObjects = vertexShaders.map((x) => compileShaderObject(gl, gl.VERTEX_SHADER, x));
  let fragmentObjects = fragmentShaders.map((x) => compileShaderObject(gl, gl.FRAGMENT_SHADER, x));

  let program = gl.createProgram();
  vertexObjects.forEach((x) => gl.attachShader(program, x));
  fragmentObjects.forEach((x) => gl.attachShader(program, x));
  gl.linkProgram(program);

  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error("Error linking shader program: " + gl.getProgramInfoLog(program));
  }

  return program;
}

class Grid {
  constructor(gridScale, rows, cols, canvas) {
	// What size is each grid cell?
	this.gridScale = gridScale;

	this.rows = rows;
	this.cols = cols;

	this.oldVelBuf = new Float32Array(rows * cols * 2);
	this.newVelBuf = new Float32Array(rows * cols * 2);
	for(let cx = 0; cx < this.cols; cx++) {
	  for(let cy = 0; cy < this.rows; cy++) {
		this.oldVelBuf[cy * this.cols * 2 + cx * 2 + 0] = cx / this.cols;
		this.oldVelBuf[cy * this.cols * 2 + cx * 2 + 1] = cy / this.rows;
	  }
	}

	this.stirrerX = 0.0;
	this.stirrerY = 0.0;
	this.stirrerRadius = 10.0;

	this.numParticles = cols-2;
	this.particlePosX = new Float32Array(this.numParticles);
	this.particlePosY = new Float32Array(this.numParticles);
	for(let i = 1; i < cols-1; i++) {
	  this.particlePosX[i] = i+0.5;
	  this.particlePosY[i] = 50;
	}

	this.oldLiquidAmountBuf = new Float32Array(rows * cols);
	this.newLiquidAmountBuf = new Float32Array(rows * cols);
	for(let cx = 1; cx < 30; cx++) {
	  for(let cy = 30; cy < 50; cy++) {
		this.oldLiquidAmountBuf[cy * this.cols + cx] = 1.0;
	  }
	}

	this.canvas = canvas;
	// this.context = canvas.getContext('2d');
	this.gl = canvas.getContext('webgl2');

	// Enable this extension so we can render to a floating-point texture.
	if(this.gl.getExtension('EXT_color_buffer_float') == null) {
	  throw new Error('Cannot load EXT_color_buffer_float extension');
	}

	// Initialize vertex data.
	this.vertexBuf = this.gl.createBuffer();
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.bufferData(this.gl.ARRAY_BUFFER, new Float32Array([
      1.0,  1.0,
      -1.0, 1.0,
      1.0,  -1.0,
      -1.0, -1.0,
    ]), this.gl.STATIC_DRAW);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	// Initialize the old velocity cells texture.
	this.oldVelocityTexture = this.gl.createTexture();
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);
	this.gl.texImage2D(this.gl.TEXTURE_2D, 0, this.gl.RG32F, this.cols, this.rows,
					   0, this.gl.RG, this.gl.FLOAT, this.oldVelBuf, 0);
	this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.gl.CLAMP_TO_EDGE);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.gl.CLAMP_TO_EDGE);
    this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	// Initialize the new velocity cells texture.
	this.newVelocityTexture = this.gl.createTexture();
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.newVelocityTexture);
	this.gl.texImage2D(this.gl.TEXTURE_2D, 0, this.gl.RG32F, this.cols, this.rows,
					   0, this.gl.RG, this.gl.FLOAT, this.newVelBuf, 0);
	this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.gl.CLAMP_TO_EDGE);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.gl.CLAMP_TO_EDGE);
    this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	// A framebuffer that sets our new velocity cells texture as the render target.
	this.velocityFB = this.gl.createFramebuffer();
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.framebufferTexture2D(this.gl.FRAMEBUFFER,
								 this.gl.COLOR_ATTACHMENT0,
								 this.gl.TEXTURE_2D,
								 this.newVelocityTexture,
								 0);
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	// Set up divergence buffer and texture.
	this.divergenceBuf = new Float32Array(rows * cols);
	this.divergenceTexture = this.gl.createTexture();
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.divergenceTexture);
	this.gl.texImage2D(this.gl.TEXTURE_2D, 0, this.gl.R32F, this.cols, this.rows,
					   0, this.gl.RED, this.gl.FLOAT, this.divergenceBuf, 0);
	this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.gl.NEAREST);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.gl.CLAMP_TO_EDGE);
    this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.gl.CLAMP_TO_EDGE);
    this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	// Set up divergence framebuffer --- allows setting the divergence texture
	// as the render target.
	this.divergenceFB = this.gl.createFramebuffer();
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.divergenceFB);
	this.gl.framebufferTexture2D(this.gl.FRAMEBUFFER,
								 this.gl.COLOR_ATTACHMENT0,
								 this.gl.TEXTURE_2D,
								 this.divergenceTexture,
								 0);
	if(this.gl.checkFramebufferStatus(this.gl.FRAMEBUFFER) != this.gl.FRAMEBUFFER_COMPLETE) {
	  throw new Error("Can't render to R32F texture");
	}
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.boundaryConditionProgram = buildShaderProgram(this.gl,
													   [doNothingVertexShader],
													   [boundaryConditionFragmentShader]);
	this.boundaryConditionVertexLoc = this.gl.getAttribLocation(this.boundaryConditionProgram, 'vertexPos');
	this.boundaryConditionStirrerPosLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'stirrerPos');
	this.boundaryConditionStirrerRadiusLoc = this.gl.getUniformLocation(this.boundaryConditionProgram, 'stirrerRadius');
	this.boundaryConditionOldVelocityLocation = this.gl.getUniformLocation(this.boundaryConditionProgram, 'oldVelocity');

	this.advectProgram = buildShaderProgram(this.gl,
											[doNothingVertexShader],
											[advectFragmentShader]);
	this.advectVertexLoc = this.gl.getAttribLocation(this.advectProgram, 'vertexPos');
	this.advectDTLoc = this.gl.getUniformLocation(this.advectProgram, 'dt');
	this.advectGridScaleLoc = this.gl.getUniformLocation(this.advectProgram, 'gridScale');
	this.advectOldVelocityLoc = this.gl.getUniformLocation(this.advectProgram,
															 'oldVelocity');

	this.computeDivergenceProgram = buildShaderProgram(this.gl,
													   [doNothingVertexShader],
													   [computeDivergenceFragmentShader]);
	this.computeDivergenceVertexLoc = this.gl.getAttribLocation(this.computeDivergenceProgram, 'vertexPos');
	this.computeDivergenceOldVelocityLoc = this.gl.getUniformLocation(this.computeDivergenceProgram, 'oldVelocity');

	this.removeDivergenceProgram = buildShaderProgram(this.gl,
													  [doNothingVertexShader],
													  [removeDivergenceFragmentShader]);
	this.removeDivergenceVertexLoc = this.gl.getAttribLocation(this.removeDivergenceProgram, 'vertexPos');
	this.removeDivergenceOldVelocityLoc = this.gl.getUniformLocation(this.removeDivergenceProgram, 'oldVelocity');
	this.removeDivergenceDivergenceLoc = this.gl.getUniformLocation(this.removeDivergenceProgram, 'divergence');

	this.renderProgram = buildShaderProgram(this.gl,
											[doNothingVertexShader],
											[renderFragmentShader]);
	this.renderVertexLoc = this.gl.getAttribLocation(this.renderProgram, 'vertexPos');
	this.renderViewportLoc = this.gl.getUniformLocation(this.renderProgram, 'viewport');
	this.renderMetersPerCellLoc = this.gl.getUniformLocation(this.renderProgram, 'metersPerCell');
	this.renderOldVelocityLoc = this.gl.getUniformLocation(this.renderProgram, 'oldVelocity');
	this.renderDivergenceLoc = this.gl.getUniformLocation(this.renderProgram, 'divergence');
  }

  swapVelocityTextures() {
	let tmp = this.oldVelocityTexture;
	this.oldVelocityTexture = this.newVelocityTexture;
	this.newVelocityTexture = tmp;

	// Update the framebuffer target as well.
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.framebufferTexture2D(this.gl.FRAMEBUFFER,
								 this.gl.COLOR_ATTACHMENT0,
								 this.gl.TEXTURE_2D,
								 this.newVelocityTexture,
								 0);
	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);
  }

  runBoundaryConditionProgram() {
	this.gl.useProgram(this.boundaryConditionProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.boundaryConditionVertexLoc);
	this.gl.vertexAttribPointer(this.boundaryConditionVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.boundaryConditionOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform2f(this.boundaryConditionStirrerPosLoc, this.stirrerX, this.stirrerY);
	this.gl.uniform1f(this.boundaryConditionStirrerRadiusLoc, 10.0);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runAdvectProgram(dt) {
	this.gl.useProgram(this.advectProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.advectVertexLoc);
	this.gl.vertexAttribPointer(this.advectVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.advectOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform1f(this.advectDTLoc, dt);

	this.gl.uniform1f(this.advectGridScaleLoc, 1.0);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runComputeDivergenceProgram() {
	this.gl.useProgram(this.computeDivergenceProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.computeDivergenceVertexLoc);
	this.gl.vertexAttribPointer(this.computeDivergenceVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.computeDivergenceOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.divergenceFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runRemoveDivergenceProgram() {
	this.gl.useProgram(this.removeDivergenceProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.removeDivergenceVertexLoc);
	this.gl.vertexAttribPointer(this.removeDivergenceVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.removeDivergenceOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform1i(this.removeDivergenceDivergenceLoc, 1);
	this.gl.activeTexture(this.gl.TEXTURE1);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.divergenceTexture);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.velocityFB);
	this.gl.viewport(0, 0, this.cols, this.rows);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null);

	this.gl.activeTexture(this.gl.TEXTURE1);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  runRenderProgram() {
	this.gl.useProgram(this.renderProgram);

	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.vertexBuf);
	this.gl.enableVertexAttribArray(this.renderVertexLoc);
	this.gl.vertexAttribPointer(this.renderVertexLoc, 2, this.gl.FLOAT, false, 0, 0);

	this.gl.uniform1i(this.renderOldVelocityLoc, 0);
	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.oldVelocityTexture);

	this.gl.uniform1i(this.renderDivergenceLoc, 1);
	this.gl.activeTexture(this.gl.TEXTURE1);
	this.gl.bindTexture(this.gl.TEXTURE_2D, this.divergenceTexture);

	this.gl.uniform2i(this.renderViewportLoc, this.canvas.width, this.canvas.height);

	this.gl.uniform1f(this.renderMetersPerCellLoc, this.gridScale);

	this.gl.viewport(0, 0, this.canvas.width, this.canvas.height);

	this.gl.drawArrays(this.gl.TRIANGLE_STRIP, 0, 4);

	this.gl.activeTexture(this.gl.TEXTURE1);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.activeTexture(this.gl.TEXTURE0);
	this.gl.bindTexture(this.gl.TEXTURE_2D, null);

	this.gl.enableVertexAttribArray(null);
	this.gl.bindBuffer(this.gl.ARRAY_BUFFER, null);

	this.gl.useProgram(null);
  }

  run() {
	// The system dynamics are always simulated with a fixed timestep, iterated
	// enough times to bring us up to the current simulation time.
	let physicsDT = 0.05;

	let curTime = performance.now() / 1000.0;
	let physicsCurTime = 0.0;

	let step = (newTime) => {
	  newTime /= 1000.0;

	  let dt = newTime - curTime;

	  // Clamp our elapsed real time, to prevent a long first frame after pausing
	  // in the debugger.
	  if(dt > 0.1) {
		dt = 0.1;
	  }

	  let physicsTargetTime = physicsCurTime + dt;
	  while(physicsCurTime < physicsTargetTime) {
		this.runBoundaryConditionProgram();
		this.swapVelocityTextures();

		this.runAdvectProgram(physicsDT);
		this.swapVelocityTextures();

		for(let i = 0; i < this.cols; i++) {
		  this.runBoundaryConditionProgram();
		  this.swapVelocityTextures();

		  this.runComputeDivergenceProgram();
		  this.runRemoveDivergenceProgram();
		  this.swapVelocityTextures();
		}

		this.stirrerX = this.cols / 2.0 + this.cols / 2.0 * Math.cos(physicsCurTime / 10.0);
		this.stirrerY = 5;

		physicsCurTime += physicsDT;
	  }

	  this.runRenderProgram();

	  curTime = newTime;
	  requestAnimationFrame(step);
	};

	requestAnimationFrame(step);
  }
}

self.onmessage = (msg) => {
  let canvas = msg.data.canvas;
  let grid = new Grid(0.1, 100, 100, canvas);
  grid.run();
}
