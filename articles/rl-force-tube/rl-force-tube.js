
// First, let's set up the physical system's dynamics.
//
// 

let ball_make = (mass, pos, vel) => ({mass: mass,
                                      pos: pos,
                                      vel: vel});

// Symplectic Euler
let ball_integrate = (ball, force, dt) => {
    let acc = force / ball.mass;
    let new_vel = ball.vel + dt * acc;
    let new_pos = ball.pos + dt * new_vel;
    return ball_make(ball.mass,
                     new_pos,
                     new_vel);
};

let gravity_force = -9.8;

let run_physics_step = (ball, agent_force, cur_time, target_time, dt) => {
    while(cur_time < target_time) {
        ball_integrate(ball, (agent_force + gravity_force), dt);
        cur_time += dt;
    }
    return [ball, cur_time];
};


// Next, set up our representation for function approximations
//
// We will use a one-dimensional dense cosine basis.

let cos = Math.cos;
let pi = Math.pi;

let cosine_basis_make_zero = (min_x, max_x, basis_size) => {
    return {min_x: min_x,
            max_x: max_x,
            basis_size: basis_size,
            basis_weights: new Float32Array(basis_size)};
};

let cosine_basis_value = (basis, x) => {
    let x_norm = (x - basis.min_value) / (basis.max_value - basis.min_value);
    
    let value = 0;
    for(let i = 0; i < basis.basis_size; ++i) {
        value += basis.basis_weights[i] * cos(pi * i * x_norm);
    }

    return value;
};

let cosine_basis_refine = (basis, x, cur_y, new_y) => {
    let x_norm = (x - basis.min_value) / (basis.max_value - basis.min_value);
    
    let delta = new_y - cur_y;
    
    let base_step = 0.005;
    for(let i = 0; i < basis.basis_size; ++i) {
        let step = base_step / (1 + i);
        basis.basis_weights[i] += step * delta * cos(pi * i * x_norm);
    }
};

// Now, define our control problem 

let ball_pos_min = 0.0;
let ball_pos_max = 0.0;

let end_episode_p = (ball) =>
    (ball.pos < 0.0) || (ball.pos > 100.0);

let reward = (ball) =>
    ((ball.pos >= 60.0) && (ball.pos <= 70.0))
    ? 10.0
    : 0.0;

let policy_make = (basis_size) => ({reward_fn_force_on: cosine_basis_make_zero(ball_pos_min,
                                                                               ball_pos_max,
                                                                               basis_size),
                                    reward_fn_force_off: cosine_basis_make_zero(ball_pos_min,
                                                                                ball_pos_max,
                                                                                basis_size)});

// Select an epsilon-greedy action according to the policy and the
// current state of the system.
let policy_apply = (policy, ball) => {
    let est_value_on = cosine_basis_value(policy.reward_fn_force_on,
                                                 ball.pos);
    let est_value_off = cosine_basis_value(policy.reward_fn_force_off,
                                                  ball.pos);

    let epsilon = 0.001;
    let trial = Math.random();
    
    if(trial < epsilon / 2) {
        return ['on', est_value_on];
    } else if(trial < epsilon) {
        return ['off', est_value_off];
    } else {
        return est_value_on > est_value_off
            ? ['on', est_value_on]
            : ['off', est_value_off];
    }
};

let run_training_episode = (policy, cur_ball, discount_rate, dt_learn, dt_phys) => {
    let reward_sum = 0.0;
    let cur_time = 0.0;

    let [cur_action, cur_est_value] = policy_apply(policy, cur_ball);
    
    while(true) {
        let agent_force = cur_action == 'on' ? 15.0 : 0.0;
        let [new_ball, new_time] = run_physics_step(ball,
                                                    agent_force,
                                                    cur_time,
                                                    cur_time + dt_learn,
                                                    dt_phys);
        let reward = reward(new_ball);
        
        if(end_episode_p(new_ball)) {
            if(cur_action == 'on') {
                cosine_basis_refine(policy.reward_fn_force_on,
                                    cur_ball.pos,
                                    cur_est_value,
                                    reward);
            } else {
                cosine_basis_refine(policy.reward_fn_force_off,
                                    cur_ball.pos,
                                    cur_est_value,
                                    reward);
            }

            return policy;
        } else {
            let [new_action, new_state_est_value] = policy_apply(policy, new_ball);
            let new_est_value = reward + discount_rate * new_state_est_value;
            
            if(new_action == 'on') {
                cosine_basis_refine(policy.reward_fn_force_on,
                                    cur_ball.pos,
                                    cur_est_value,
                                    new_est_value);
            } else {
                cosine_basis_refine(policy.reward_fn_force_off,
                                    cur_ball.pos,
                                    cur_est_value,
                                    new_est_value);
            }
            
            cur_action = new_action;
            cur_time = new_time;
        }
    }
}

let main = () => {

    let action_value_fn_plot = d3
        .select('#action-value-function')
        .append('svg')
        .attr('width', '100%')
        .attr('height', '100%');

    let circle = action_value_fn_plot
        .append('circle')
        .attr('r', 100)
        .attr('fill', 'rgba(1.0, 0.0, 0.0, 1.0)');

    let cur_time = performance.now();
    let real_time_step = (new_time) => {
        let real_dt = new_time - cur_time;
        
        cur_time = new_time;

        window.requestAnimationFrame(real_time_step);
    };

    window.requestAnimationFrame(real_time_step);
    
};

document.addEventListener('DOMContentLoaded', (event) => main());



