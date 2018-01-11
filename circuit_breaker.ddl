metadata    :name        => "circuit_breaker",
            :description => "Circuit Breaker Switcher",
            :author      => "R.I.Pienaar <rip@devco.net>",
            :license     => "Apache License, Version 2.0",
            :version     => "0.0.1",
            :url         => "https://choria.io/",
            :timeout     => 2

action "switch", :description => "Flip the switch into opposite state" do
    display :always
end
